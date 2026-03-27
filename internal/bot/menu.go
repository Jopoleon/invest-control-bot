package bot

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/channelurl"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

const (
	menuCallbackPrefix        = "menu:"
	menuCallbackSubscription  = "menu:subscription"
	menuCallbackPayments      = "menu:payments"
	menuCallbackAutopay       = "menu:autopay"
	menuCallbackAutopayPick   = "menu:autopay:pick"
	menuCallbackAutopayOn     = "menu:autopay:on"
	menuCallbackAutopayOnSub  = "menu:autopay:on:sub:"
	menuCallbackAutopayOffSub = "menu:autopay:off:sub:"
	menuCallbackAutopayOff    = "menu:autopay:off"
	menuCallbackAutopayOffAsk = "menu:autopay:off:ask"
	menuCallbackAutopayOffNo  = "menu:autopay:off:no"
)

func (h *Handler) sendMenu(ctx context.Context, chatID int64) {
	rows := [][]messenger.ActionButton{
		{
			buttonAction(botMenuButtonSubscription, menuCallbackSubscription),
		},
		{
			buttonAction(botMenuButtonPayments, menuCallbackPayments),
		},
	}
	if h.recurringEnabled {
		rows = append(rows, []messenger.ActionButton{
			buttonAction(botMenuButtonAutopay, menuCallbackAutopay),
		})
	}
	if err := h.sender.Send(ctx, chatRef(chatID), messenger.OutgoingMessage{Text: botMenuTitle, Buttons: rows}); err != nil {
		slog.Error("send menu failed", "error", err, "chat_id", chatID)
	}
}

func (h *Handler) handleMenuCallback(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 {
		return
	}
	chatID := cb.ChatID
	if strings.HasPrefix(cb.Data, menuCallbackAutopayOnSub) {
		h.reactivateAutopayForSubscription(ctx, cb)
		return
	}
	if strings.HasPrefix(cb.Data, menuCallbackAutopayOffSub) {
		h.disableAutopayForSubscription(ctx, cb)
		return
	}

	switch cb.Data {
	case menuCallbackSubscription:
		h.sendSubscriptionOverview(ctx, chatID, cb.User.ID)
	case menuCallbackPayments:
		h.sendPaymentHistory(ctx, chatID, cb.User.ID)
	case menuCallbackAutopay:
		h.sendAutopayInfo(ctx, chatID, cb.User.ID)
	case menuCallbackAutopayPick:
		h.showAutopaySubscriptionChooser(ctx, cb)
	case menuCallbackAutopayOn:
		h.setAutopayPreference(ctx, chatID, cb.User.ID, true)
	case menuCallbackAutopayOffAsk:
		h.confirmAutopayDisable(ctx, cb)
	case menuCallbackAutopayOff:
		h.disableAutopayConfirmed(ctx, cb)
	case menuCallbackAutopayOffNo:
		h.restoreAutopayInfo(ctx, cb)
	default:
		h.send(ctx, chatID, botMsgUnknownMenuCommand)
	}
}

func (h *Handler) sendSubscriptionOverview(ctx context.Context, chatID, telegramID int64) {
	subs, err := h.store.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		TelegramID: telegramID,
		Status:     domain.SubscriptionStatusActive,
		Limit:      20,
	})
	if err != nil {
		slog.Error("list active subscriptions failed", "error", err, "telegram_id", telegramID)
		h.send(ctx, chatID, botMsgSubscriptionLoadFailed)
		return
	}

	if len(subs) == 0 {
		h.send(ctx, chatID, botMsgNoActiveSubscriptions)
		return
	}

	lines := []string{botMenuSubscriptionHeader, ""}
	for _, sub := range subs {
		connector, ok, err := h.store.GetConnector(ctx, sub.ConnectorID)
		if err != nil {
			slog.Error("load connector for subscription overview failed", "error", err, "connector_id", sub.ConnectorID, "telegram_id", telegramID)
			continue
		}
		if !ok {
			continue
		}

		channel := resolveChannelForBot(connector.ChannelURL, connector.ChatID)
		lines = append(lines, botSubscriptionOverviewLines(sub, connector, channel)...)
	}
	if len(lines) <= 2 {
		h.send(ctx, chatID, botMsgNoActiveSubscriptions)
		return
	}

	h.send(ctx, chatID, strings.TrimSpace(strings.Join(lines, "\n")))
	h.logAuditEvent(ctx, telegramID, 0, domain.AuditActionMenuSubscriptionOpened, "")
}

func (h *Handler) sendPaymentHistory(ctx context.Context, chatID, telegramID int64) {
	payments, err := h.store.ListPayments(ctx, domain.PaymentListQuery{
		TelegramID: telegramID,
		Limit:      5,
	})
	if err != nil {
		slog.Error("list payments for menu failed", "error", err, "telegram_id", telegramID)
		h.send(ctx, chatID, botMsgPaymentsLoadFailed)
		return
	}
	if len(payments) == 0 {
		h.send(ctx, chatID, botMsgNoPayments)
		return
	}

	lines := []string{botMenuPaymentsHeader, ""}
	for _, p := range payments {
		lines = append(lines, botPaymentHistoryLines(p)...)
	}
	h.send(ctx, chatID, strings.Join(lines, "\n"))
	h.logAuditEvent(ctx, telegramID, 0, domain.AuditActionMenuPaymentsOpened, "")
}

func (h *Handler) sendAutopayInfo(ctx context.Context, chatID, telegramID int64) {
	if !h.recurringEnabled {
		h.send(ctx, chatID, botMsgAutopayUnavailable)
		return
	}
	options := h.listAutopayOptions(ctx, telegramID)
	enabledCount := 0
	for _, option := range options {
		if option.AutoPayEnabled {
			enabledCount++
		}
	}
	text, keyboard := botAutopayInfoMessage(enabledCount, len(options), h.buildAutopayCancelURL(telegramID))
	if err := h.sender.Send(ctx, chatRef(chatID), messenger.OutgoingMessage{Text: text, Buttons: keyboard}); err != nil {
		slog.Error("send autopay info failed", "error", err, "telegram_id", telegramID)
		return
	}
	h.logAuditEvent(ctx, telegramID, 0, domain.AuditActionMenuAutoPayOpened, "")
}

func (h *Handler) confirmAutopayDisable(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 {
		return
	}
	text := botMsgAutopayDisableConfirm
	keyboard := [][]messenger.ActionButton{
		{
			buttonAction(botBtnAutopayDisableConfirm, menuCallbackAutopayOff),
			buttonAction(botBtnAutopayDisableCancel, menuCallbackAutopayOffNo),
		},
	}
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
	options := h.listAutopayOptions(ctx, cb.User.ID)
	enabledCount := 0
	for _, option := range options {
		if option.AutoPayEnabled {
			enabledCount++
		}
	}
	text, keyboard := botAutopayInfoMessage(enabledCount, len(options), h.buildAutopayCancelURL(cb.User.ID))
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

func (h *Handler) listAutopayOptions(ctx context.Context, telegramID int64) []autopayOption {
	subs, err := h.store.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		TelegramID: telegramID,
		Status:     domain.SubscriptionStatusActive,
		Limit:      20,
	})
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
	rows := make([][]messenger.ActionButton, 0, len(options)+1)
	for _, option := range options {
		if option.AutoPayEnabled {
			rows = append(rows, []messenger.ActionButton{{
				Text:   botAutopayChooserButtonDisable(option.Name),
				Action: menuCallbackAutopayOffSub + strconv.FormatInt(option.SubscriptionID, 10),
			}})
			continue
		}
		if option.Reactivatable {
			rows = append(rows, []messenger.ActionButton{{
				Text:   botAutopayChooserButtonReenable(option.Name),
				Action: menuCallbackAutopayOnSub + strconv.FormatInt(option.SubscriptionID, 10),
			}})
			continue
		}
		url := h.buildAutopayCheckoutURL(option.ConnectorID)
		if url == "" {
			continue
		}
		rows = append(rows, []messenger.ActionButton{{
			Text: botAutopayChooserButtonCheckout(option.Name),
			URL:  url,
		}})
	}
	rows = append(rows, []messenger.ActionButton{{
		Text:   botBtnBack,
		Action: menuCallbackAutopay,
	}})
	if err := h.respondToAction(ctx, cb, messenger.OutgoingMessage{Text: botMsgAutopayChooser, Buttons: rows}); err != nil {
		slog.Error("show autopay subscription chooser failed", "error", err, "telegram_id", cb.User.ID)
	}
}

func (h *Handler) reactivateAutopayForSubscription(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 {
		return
	}
	subIDRaw := strings.TrimPrefix(cb.Data, menuCallbackAutopayOnSub)
	subID, err := strconv.ParseInt(subIDRaw, 10, 64)
	if err != nil || subID <= 0 {
		h.send(ctx, cb.ChatID, botMsgAutopayReactivationResolveFailed)
		return
	}
	sub, found, err := h.store.GetSubscriptionByID(ctx, subID)
	if err != nil || !found || sub.TelegramID != cb.User.ID {
		h.send(ctx, cb.ChatID, botMsgAutopaySubscriptionNotFound)
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
	subIDRaw := strings.TrimPrefix(cb.Data, menuCallbackAutopayOffSub)
	subID, err := strconv.ParseInt(subIDRaw, 10, 64)
	if err != nil || subID <= 0 {
		h.send(ctx, cb.ChatID, botMsgAutopayDisableResolveFailed)
		return
	}
	sub, found, err := h.store.GetSubscriptionByID(ctx, subID)
	if err != nil || !found || sub.TelegramID != cb.User.ID {
		h.send(ctx, cb.ChatID, botMsgAutopaySubscriptionNotFound)
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
	connectorName := ""
	if connector, found, err := h.store.GetConnector(ctx, sub.ConnectorID); err == nil && found {
		connectorName = connector.Name
	}
	h.logAuditEvent(ctx, cb.User.ID, sub.ConnectorID, domain.AuditActionAutopayDisabled, "source=bot_menu;subscription_id="+strconv.FormatInt(sub.ID, 10))
	if err := h.respondToAction(ctx, cb, messenger.OutgoingMessage{Text: botAutopayDisabledForSubscription(connectorName)}); err != nil {
		slog.Error("render autopay disabled per subscription message failed", "error", err, "telegram_id", cb.User.ID)
	}
}

func (h *Handler) syncUserAutoPayPreference(ctx context.Context, telegramID int64, now time.Time) {
	options := h.listAutopayOptions(ctx, telegramID)
	enabled := false
	for _, option := range options {
		if option.AutoPayEnabled {
			enabled = true
			break
		}
	}
	if err := h.store.SetUserAutoPayEnabled(ctx, telegramID, enabled, now); err != nil {
		slog.Error("sync user autopay preference failed", "error", err, "telegram_id", telegramID, "enabled", enabled)
	}
}

func resolveChannelForBot(channelURL, chatID string) string {
	return channelurl.Resolve(channelURL, chatID)
}
