package bot

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

func (h *Handler) sendAutopayInfo(ctx context.Context, chatID int64, userIdentity messenger.UserIdentity) {
	if !h.recurringEnabled {
		h.sendTo(ctx, chatID, userIdentity, botMsgAutopayUnavailable)
		return
	}
	text, keyboard := h.buildAutopayInfo(ctx, userIdentity)
	if err := h.sender.Send(ctx, recipientRef(chatID, userIdentity), messenger.OutgoingMessage{Text: text, Buttons: keyboard}); err != nil {
		slog.Error("send autopay info failed", "error", err, "messenger_kind", userIdentity.Kind, "messenger_user_id", userIdentity.ID)
		return
	}
	h.logAuditEvent(ctx, userIdentity, 0, domain.AuditActionMenuAutoPayOpened, "")
}

func (h *Handler) buildAutopayInfo(ctx context.Context, userIdentity messenger.UserIdentity) (string, [][]messenger.ActionButton) {
	options := h.listAutopayOptions(ctx, userIdentity)
	enabledCount := countEnabledAutopayOptions(options)
	return botAutopayInfoMessage(enabledCount, len(options), h.buildAutopayCancelURL(userIdentity.ID))
}

func countEnabledAutopayOptions(options []autopayOption) int {
	enabledCount := 0
	for _, option := range options {
		if option.AutoPayEnabled {
			enabledCount++
		}
	}
	return enabledCount
}

func (h *Handler) confirmAutopayDisable(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 {
		return
	}
	text := botMsgAutopayDisableConfirm
	keyboard := [][]messenger.ActionButton{{
		buttonAction(botBtnAutopayDisableConfirm, menuCallbackAutopayOff),
		buttonAction(botBtnAutopayDisableCancel, menuCallbackAutopayOffNo),
	}}
	if err := h.respondToAction(ctx, cb, messenger.OutgoingMessage{Text: text, Buttons: keyboard}); err != nil {
		slog.Error("render autopay disable confirm failed", "error", err, "messenger_kind", cb.User.Kind, "messenger_user_id", cb.User.ID)
		return
	}
	h.logAuditEvent(ctx, cb.User, 0, domain.AuditActionAutopayDisableRequested, "")
}

func (h *Handler) restoreAutopayInfo(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 {
		return
	}
	text, keyboard := h.buildAutopayInfo(ctx, cb.User)
	if err := h.respondToAction(ctx, cb, messenger.OutgoingMessage{Text: text, Buttons: keyboard}); err != nil {
		slog.Error("restore autopay info failed", "error", err, "messenger_kind", cb.User.Kind, "messenger_user_id", cb.User.ID)
	}
}

func (h *Handler) setAutopayPreference(ctx context.Context, chatID int64, userIdentity messenger.UserIdentity, enabled bool) {
	if !h.recurringEnabled {
		h.sendTo(ctx, chatID, userIdentity, botMsgAutopayUnavailable)
		return
	}
	if enabled {
		h.sendTo(ctx, chatID, userIdentity, botMsgAutopayEnableOnlyDuringPayment)
		return
	}
	h.sendTo(ctx, chatID, userIdentity, botMsgAutopayDisabledShort)
	h.logAuditEvent(ctx, userIdentity, 0, domain.AuditActionAutopayDisabled, "")
}

func (h *Handler) disableAutopayConfirmed(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 {
		return
	}
	if !h.recurringEnabled {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopayUnavailable)
		return
	}
	user, resolved := h.resolveMessengerUser(ctx, cb.User)
	if !resolved {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopayDisableSubscriptionsFailed)
		return
	}
	if err := h.store.DisableAutoPayForActiveSubscriptions(ctx, user.ID, time.Now().UTC()); err != nil {
		slog.Error("disable subscription autopay failed", "error", err, "messenger_kind", cb.User.Kind, "messenger_user_id", cb.User.ID)
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopayDisableSubscriptionsFailed)
		return
	}
	if err := h.respondToAction(ctx, cb, messenger.OutgoingMessage{Text: botMsgAutopayDisabled}); err != nil {
		slog.Error("render autopay disabled message failed", "error", err, "messenger_kind", cb.User.Kind, "messenger_user_id", cb.User.ID)
		return
	}
	h.logAuditEvent(ctx, cb.User, 0, domain.AuditActionAutopayDisabled, "")
}

type autopayOption struct {
	SubscriptionID int64
	ConnectorID    int64
	Name           string
	AutoPayEnabled bool
	Reactivatable  bool
}

// listAutopayOptions resolves actionable menu choices from active subscriptions.
// The menu needs more than raw subscription rows: connector names for labels and
// parent payment flags to decide whether re-enable is allowed without a new payment.
func (h *Handler) listAutopayOptions(ctx context.Context, userIdentity messenger.UserIdentity) []autopayOption {
	subs, err := queryActiveSubscriptions(ctx, h, userIdentity)
	if err != nil {
		slog.Error("list subscriptions for autopay options failed", "error", err, "messenger_kind", userIdentity.Kind, "messenger_user_id", userIdentity.ID)
		return nil
	}
	manageableSubs := subscriptionsForAutopayMenu(subs, time.Now().UTC())
	options := make([]autopayOption, 0, len(manageableSubs))
	for _, sub := range manageableSubs {
		connector, found, err := h.store.GetConnector(ctx, sub.ConnectorID)
		if err != nil || !found {
			continue
		}
		paymentRow, found, err := h.store.GetPaymentByID(ctx, sub.PaymentID)
		if err != nil || !found {
			continue
		}
		options = append(options, autopayOption{
			SubscriptionID: sub.ID,
			ConnectorID:    sub.ConnectorID,
			Name:           connector.Name,
			AutoPayEnabled: sub.AutoPayEnabled,
			Reactivatable:  !sub.AutoPayEnabled && paymentRow.AutoPayEnabled,
		})
	}
	return options
}

func subscriptionsForAutopayMenu(subs []domain.Subscription, now time.Time) []domain.Subscription {
	current, future := splitCurrentAndFutureSubscriptions(subs, now)
	if len(current) > 0 {
		return current
	}
	return future
}

func (h *Handler) showAutopaySubscriptionChooser(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 {
		return
	}
	options := h.listAutopayOptions(ctx, cb.User)
	if len(options) == 0 {
		h.restoreAutopayInfo(ctx, cb)
		return
	}
	rows := buildAutopayChooserRows(options, h.buildAutopayCheckoutURL)
	rows = append(rows, []messenger.ActionButton{{
		Text:   botBtnBack,
		Action: menuCallbackAutopay,
	}})
	if err := h.respondToAction(ctx, cb, messenger.OutgoingMessage{Text: botMsgAutopayChooser, Buttons: rows}); err != nil {
		slog.Error("show autopay subscription chooser failed", "error", err, "messenger_kind", cb.User.Kind, "messenger_user_id", cb.User.ID)
	}
}

func buildAutopayChooserRows(options []autopayOption, checkoutURL func(int64) string) [][]messenger.ActionButton {
	rows := make([][]messenger.ActionButton, 0, len(options)+1)
	for _, option := range options {
		switch {
		case option.AutoPayEnabled:
			rows = append(rows, []messenger.ActionButton{{
				Text:   botAutopayChooserButtonDisable(option.Name),
				Action: menuCallbackAutopayOffSub + strconv.FormatInt(option.SubscriptionID, 10),
			}})
		case option.Reactivatable:
			rows = append(rows, []messenger.ActionButton{{
				Text:   botAutopayChooserButtonReenable(option.Name),
				Action: menuCallbackAutopayOnSub + strconv.FormatInt(option.SubscriptionID, 10),
			}})
		default:
			url := checkoutURL(option.ConnectorID)
			if url == "" {
				continue
			}
			rows = append(rows, []messenger.ActionButton{{
				Text: botAutopayChooserButtonCheckout(option.Name),
				URL:  url,
			}})
		}
	}
	return rows
}

func (h *Handler) reactivateAutopayForSubscription(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 {
		return
	}
	sub, ok := h.loadOwnedSubscriptionFromMenuAction(ctx, cb, menuCallbackAutopayOnSub, botMsgAutopayReactivationResolveFailed)
	if !ok {
		return
	}
	if sub.Status != domain.SubscriptionStatusActive {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopayReactivationOnlyForActive)
		return
	}
	if sub.AutoPayEnabled {
		h.restoreAutopayInfo(ctx, cb)
		return
	}
	paymentRow, found, err := h.store.GetPaymentByID(ctx, sub.PaymentID)
	if err != nil || !found || !paymentRow.AutoPayEnabled {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopayReactivationUnavailable)
		return
	}
	connector, found, err := h.store.GetConnector(ctx, sub.ConnectorID)
	if err != nil || !found {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgTariffNotFound)
		return
	}
	if !connector.SupportsRecurring() {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopayReactivationUnavailable)
		return
	}
	user, resolved := h.resolveMessengerUser(ctx, cb.User)
	if !resolved {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopayConsentConfirmFailed)
		return
	}
	recurringConsent, consentErr := h.buildRecurringConsent(ctx, user.ID, connector)
	if consentErr != nil {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopayConsentConfirmFailed)
		return
	}
	if err := h.store.CreateRecurringConsent(ctx, recurringConsent); err != nil {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopayConsentPersistFailed)
		return
	}
	now := time.Now().UTC()
	if err := h.store.SetSubscriptionAutoPayEnabled(ctx, sub.ID, true, now); err != nil {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopayReactivationFailed)
		return
	}
	h.logAuditEvent(ctx, cb.User, sub.ConnectorID, domain.AuditActionRecurringConsentGranted, "source=autopay_reactivate")
	h.logAuditEvent(ctx, cb.User, sub.ConnectorID, domain.AuditActionAutopayEnabled, "source=autopay_reactivate;subscription_id="+strconv.FormatInt(sub.ID, 10))
	if err := h.respondToAction(ctx, cb, messenger.OutgoingMessage{Text: botMsgAutopayReactivated}); err != nil {
		slog.Error("render autopay reactivated message failed", "error", err, "messenger_kind", cb.User.Kind, "messenger_user_id", cb.User.ID)
	}
}

func (h *Handler) disableAutopayForSubscription(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 {
		return
	}
	sub, ok := h.loadOwnedSubscriptionFromMenuAction(ctx, cb, menuCallbackAutopayOffSub, botMsgAutopayDisableResolveFailed)
	if !ok {
		return
	}
	if sub.Status != domain.SubscriptionStatusActive {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopayDisableOnlyForActive)
		return
	}
	if !sub.AutoPayEnabled {
		h.restoreAutopayInfo(ctx, cb)
		return
	}
	now := time.Now().UTC()
	if err := h.store.SetSubscriptionAutoPayEnabled(ctx, sub.ID, false, now); err != nil {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopayDisablePerSubscriptionFailed)
		return
	}
	connectorName := h.lookupConnectorName(ctx, sub.ConnectorID)
	h.logAuditEvent(ctx, cb.User, sub.ConnectorID, domain.AuditActionAutopayDisabled, "source=bot_menu;subscription_id="+strconv.FormatInt(sub.ID, 10))
	if err := h.respondToAction(ctx, cb, messenger.OutgoingMessage{Text: botAutopayDisabledForSubscription(connectorName)}); err != nil {
		slog.Error("render autopay disabled per subscription message failed", "error", err, "messenger_kind", cb.User.Kind, "messenger_user_id", cb.User.ID)
	}
}

func (h *Handler) loadOwnedSubscriptionFromMenuAction(ctx context.Context, cb messenger.IncomingAction, prefix, invalidMsg string) (domain.Subscription, bool) {
	subIDRaw := strings.TrimPrefix(cb.Data, prefix)
	subID, err := strconv.ParseInt(subIDRaw, 10, 64)
	if err != nil || subID <= 0 {
		h.sendTo(ctx, cb.ChatID, cb.User, invalidMsg)
		return domain.Subscription{}, false
	}
	user, resolved := h.resolveMessengerUser(ctx, cb.User)
	if !resolved {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopaySubscriptionNotFound)
		return domain.Subscription{}, false
	}
	sub, found, err := h.store.GetSubscriptionByID(ctx, subID)
	if err != nil || !found || sub.UserID != user.ID {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopaySubscriptionNotFound)
		return domain.Subscription{}, false
	}
	return sub, true
}

func (h *Handler) lookupConnectorName(ctx context.Context, connectorID int64) string {
	connector, found, err := h.store.GetConnector(ctx, connectorID)
	if err != nil || !found {
		return ""
	}
	return connector.Name
}
