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

func (h *Handler) sendAutopayInfo(ctx context.Context, chatID, telegramID int64) {
	if !h.recurringEnabled {
		h.send(ctx, chatID, botMsgAutopayUnavailable)
		return
	}
	text, keyboard := h.buildAutopayInfo(ctx, telegramID)
	if err := h.sender.Send(ctx, chatRef(chatID), messenger.OutgoingMessage{Text: text, Buttons: keyboard}); err != nil {
		slog.Error("send autopay info failed", "error", err, "telegram_id", telegramID)
		return
	}
	h.logAuditEvent(ctx, telegramID, 0, domain.AuditActionMenuAutoPayOpened, "")
}

func (h *Handler) buildAutopayInfo(ctx context.Context, telegramID int64) (string, [][]messenger.ActionButton) {
	options := h.listAutopayOptions(ctx, telegramID)
	enabledCount := countEnabledAutopayOptions(options)
	return botAutopayInfoMessage(enabledCount, len(options), h.buildAutopayCancelURL(telegramID))
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
		slog.Error("render autopay disable confirm failed", "error", err, "telegram_id", cb.User.ID)
		return
	}
	h.logAuditEvent(ctx, cb.User.ID, 0, domain.AuditActionAutopayDisableRequested, "")
}

func (h *Handler) restoreAutopayInfo(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 {
		return
	}
	text, keyboard := h.buildAutopayInfo(ctx, cb.User.ID)
	if err := h.respondToAction(ctx, cb, messenger.OutgoingMessage{Text: text, Buttons: keyboard}); err != nil {
		slog.Error("restore autopay info failed", "error", err, "telegram_id", cb.User.ID)
	}
}

func (h *Handler) setAutopayPreference(ctx context.Context, chatID, telegramID int64, enabled bool) {
	if !h.recurringEnabled {
		h.send(ctx, chatID, botMsgAutopayUnavailable)
		return
	}
	if enabled {
		h.send(ctx, chatID, botMsgAutopayEnableOnlyDuringPayment)
		return
	}
	if err := h.store.SetUserAutoPayEnabled(ctx, telegramID, enabled, time.Now().UTC()); err != nil {
		slog.Error("save user autopay preference failed", "error", err, "telegram_id", telegramID, "enabled", enabled)
		h.send(ctx, chatID, botMsgAutopayPreferenceUpdateFailed)
		return
	}
	h.send(ctx, chatID, botMsgAutopayDisabledShort)
	h.logAuditEvent(ctx, telegramID, 0, domain.AuditActionAutopayDisabled, "")
}

func (h *Handler) disableAutopayConfirmed(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 {
		return
	}
	if !h.recurringEnabled {
		h.send(ctx, cb.ChatID, botMsgAutopayUnavailable)
		return
	}
	if err := h.store.SetUserAutoPayEnabled(ctx, cb.User.ID, false, time.Now().UTC()); err != nil {
		slog.Error("disable autopay failed", "error", err, "telegram_id", cb.User.ID)
		h.send(ctx, cb.ChatID, botMsgAutopayPreferenceUpdateFailed)
		return
	}
	if err := h.store.DisableAutoPayForActiveSubscriptions(ctx, cb.User.ID, time.Now().UTC()); err != nil {
		slog.Error("disable subscription autopay failed", "error", err, "telegram_id", cb.User.ID)
		h.send(ctx, cb.ChatID, botMsgAutopayDisableSubscriptionsFailed)
		return
	}
	if err := h.respondToAction(ctx, cb, messenger.OutgoingMessage{Text: botMsgAutopayDisabled}); err != nil {
		slog.Error("render autopay disabled message failed", "error", err, "telegram_id", cb.User.ID)
		return
	}
	h.logAuditEvent(ctx, cb.User.ID, 0, domain.AuditActionAutopayDisabled, "")
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
func (h *Handler) listAutopayOptions(ctx context.Context, telegramID int64) []autopayOption {
	subs, err := queryActiveSubscriptions(ctx, h, telegramID)
	if err != nil {
		slog.Error("list subscriptions for autopay options failed", "error", err, "telegram_id", telegramID)
		return nil
	}
	options := make([]autopayOption, 0, len(subs))
	for _, sub := range subs {
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

func (h *Handler) showAutopaySubscriptionChooser(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 {
		return
	}
	options := h.listAutopayOptions(ctx, cb.User.ID)
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
		slog.Error("show autopay subscription chooser failed", "error", err, "telegram_id", cb.User.ID)
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
		h.send(ctx, cb.ChatID, botMsgAutopayReactivationOnlyForActive)
		return
	}
	if sub.AutoPayEnabled {
		h.restoreAutopayInfo(ctx, cb)
		return
	}
	paymentRow, found, err := h.store.GetPaymentByID(ctx, sub.PaymentID)
	if err != nil || !found || !paymentRow.AutoPayEnabled {
		h.send(ctx, cb.ChatID, botMsgAutopayReactivationUnavailable)
		return
	}
	connector, found, err := h.store.GetConnector(ctx, sub.ConnectorID)
	if err != nil || !found {
		h.send(ctx, cb.ChatID, botMsgTariffNotFound)
		return
	}
	recurringConsent, consentErr := h.buildRecurringConsent(ctx, cb.User.ID, connector)
	if consentErr != nil {
		h.send(ctx, cb.ChatID, botMsgAutopayConsentConfirmFailed)
		return
	}
	if err := h.store.CreateRecurringConsent(ctx, recurringConsent); err != nil {
		h.send(ctx, cb.ChatID, botMsgAutopayConsentPersistFailed)
		return
	}
	now := time.Now().UTC()
	if err := h.store.SetSubscriptionAutoPayEnabled(ctx, sub.ID, true, now); err != nil {
		h.send(ctx, cb.ChatID, botMsgAutopayReactivationFailed)
		return
	}
	h.syncUserAutoPayPreference(ctx, cb.User.ID, now)
	h.logAuditEvent(ctx, cb.User.ID, sub.ConnectorID, domain.AuditActionRecurringConsentGranted, "source=autopay_reactivate")
	h.logAuditEvent(ctx, cb.User.ID, sub.ConnectorID, domain.AuditActionAutopayEnabled, "source=autopay_reactivate;subscription_id="+strconv.FormatInt(sub.ID, 10))
	if err := h.respondToAction(ctx, cb, messenger.OutgoingMessage{Text: botMsgAutopayReactivated}); err != nil {
		slog.Error("render autopay reactivated message failed", "error", err, "telegram_id", cb.User.ID)
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
		h.send(ctx, cb.ChatID, botMsgAutopayDisableOnlyForActive)
		return
	}
	if !sub.AutoPayEnabled {
		h.restoreAutopayInfo(ctx, cb)
		return
	}
	now := time.Now().UTC()
	if err := h.store.SetSubscriptionAutoPayEnabled(ctx, sub.ID, false, now); err != nil {
		h.send(ctx, cb.ChatID, botMsgAutopayDisablePerSubscriptionFailed)
		return
	}
	h.syncUserAutoPayPreference(ctx, cb.User.ID, now)
	connectorName := h.lookupConnectorName(ctx, sub.ConnectorID)
	h.logAuditEvent(ctx, cb.User.ID, sub.ConnectorID, domain.AuditActionAutopayDisabled, "source=bot_menu;subscription_id="+strconv.FormatInt(sub.ID, 10))
	if err := h.respondToAction(ctx, cb, messenger.OutgoingMessage{Text: botAutopayDisabledForSubscription(connectorName)}); err != nil {
		slog.Error("render autopay disabled per subscription message failed", "error", err, "telegram_id", cb.User.ID)
	}
}

func (h *Handler) loadOwnedSubscriptionFromMenuAction(ctx context.Context, cb messenger.IncomingAction, prefix, invalidMsg string) (domain.Subscription, bool) {
	subIDRaw := strings.TrimPrefix(cb.Data, prefix)
	subID, err := strconv.ParseInt(subIDRaw, 10, 64)
	if err != nil || subID <= 0 {
		h.send(ctx, cb.ChatID, invalidMsg)
		return domain.Subscription{}, false
	}
	sub, found, err := h.store.GetSubscriptionByID(ctx, subID)
	if err != nil || !found || sub.TelegramID != cb.User.ID {
		h.send(ctx, cb.ChatID, botMsgAutopaySubscriptionNotFound)
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

func (h *Handler) syncUserAutoPayPreference(ctx context.Context, telegramID int64, now time.Time) {
	options := h.listAutopayOptions(ctx, telegramID)
	enabled := countEnabledAutopayOptions(options) > 0
	if err := h.store.SetUserAutoPayEnabled(ctx, telegramID, enabled, now); err != nil {
		slog.Error("sync user autopay preference failed", "error", err, "telegram_id", telegramID, "enabled", enabled)
	}
}
