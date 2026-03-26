package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

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
			buttonAction("📄 Моя подписка", menuCallbackSubscription),
		},
		{
			buttonAction("💳 Платежи", menuCallbackPayments),
		},
	}
	if h.recurringEnabled {
		rows = append(rows, []messenger.ActionButton{
			buttonAction("🔁 Автоплатеж", menuCallbackAutopay),
		})
	}
	if err := h.sender.Send(ctx, chatRef(chatID), messenger.OutgoingMessage{Text: "Личный кабинет:", Buttons: rows}); err != nil {
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
		h.send(ctx, chatID, "Неизвестная команда меню.")
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
		h.send(ctx, chatID, "Не удалось загрузить подписку. Попробуйте позже.")
		return
	}

	if len(subs) == 0 {
		h.send(ctx, chatID, "У вас нет активных подписок.")
		return
	}

	lines := []string{"📄 Моя подписка", ""}
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
		lines = append(lines, fmt.Sprintf("• %s", connector.Name))
		lines = append(lines, fmt.Sprintf("  Сумма: %d ₽", connector.PriceRUB))
		lines = append(lines, fmt.Sprintf("  Период: %d дн.", connector.PeriodDays))
		lines = append(lines, fmt.Sprintf("  Действует до: %s", sub.EndsAt.In(time.Local).Format("02.01.2006 15:04")))
		if sub.AutoPayEnabled {
			lines = append(lines, "  Автоплатеж: включен")
		} else {
			lines = append(lines, "  Автоплатеж: выключен")
		}
		if channel != "" {
			lines = append(lines, fmt.Sprintf("  Канал: %s", channel))
		}
		lines = append(lines, "")
	}
	if len(lines) <= 2 {
		h.send(ctx, chatID, "У вас нет активных подписок.")
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
		h.send(ctx, chatID, "Не удалось загрузить платежи. Попробуйте позже.")
		return
	}
	if len(payments) == 0 {
		h.send(ctx, chatID, "У вас пока нет платежей.")
		return
	}

	lines := []string{"💳 Последние платежи", ""}
	for _, p := range payments {
		paid := "-"
		if p.PaidAt != nil {
			paid = p.PaidAt.In(time.Local).Format("02.01.2006 15:04")
		}
		lines = append(lines, fmt.Sprintf("• #%d — %d ₽ — %s", p.ID, p.AmountRUB, strings.ToUpper(string(p.Status))))
		lines = append(lines, fmt.Sprintf("  Создан: %s", p.CreatedAt.In(time.Local).Format("02.01.2006 15:04")))
		lines = append(lines, fmt.Sprintf("  Оплачен: %s", paid))
	}
	h.send(ctx, chatID, strings.Join(lines, "\n"))
	h.logAuditEvent(ctx, telegramID, 0, domain.AuditActionMenuPaymentsOpened, "")
}

func (h *Handler) sendAutopayInfo(ctx context.Context, chatID, telegramID int64) {
	if !h.recurringEnabled {
		h.send(ctx, chatID, "Автоплатеж временно недоступен. Подключим его после активации recurring в Robokassa.")
		return
	}
	options := h.listAutopayOptions(ctx, telegramID)
	enabledCount := 0
	for _, option := range options {
		if option.AutoPayEnabled {
			enabledCount++
		}
	}
	text, keyboard := autopayInfoMessage(enabledCount, len(options), h.buildAutopayCancelURL(telegramID))
	if err := h.sender.Send(ctx, chatRef(chatID), messenger.OutgoingMessage{Text: text, Buttons: keyboard}); err != nil {
		slog.Error("send autopay info failed", "error", err, "telegram_id", telegramID)
		return
	}
	h.logAuditEvent(ctx, telegramID, 0, domain.AuditActionMenuAutoPayOpened, "")
}

func (h *Handler) confirmAutopayDisable(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 || cb.MessageID == 0 {
		return
	}
	text := "🔁 Отключить автоплатеж?\n\nПосле отключения новые автоматические списания выполняться не будут. Текущий оплаченный период сохранится до даты окончания подписки."
	keyboard := [][]messenger.ActionButton{
		{
			buttonAction("Да, отключить", menuCallbackAutopayOff),
			buttonAction("Нет, оставить", menuCallbackAutopayOffNo),
		},
	}
	if err := h.sender.Edit(ctx, messageRef(cb.ChatID, cb.MessageID), messenger.OutgoingMessage{Text: text, Buttons: keyboard}); err != nil {
		slog.Error("edit autopay disable confirm failed", "error", err, "telegram_id", cb.User.ID)
		return
	}
	h.logAuditEvent(ctx, cb.User.ID, 0, domain.AuditActionAutopayDisableRequested, "")
}

func (h *Handler) restoreAutopayInfo(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 || cb.MessageID == 0 {
		return
	}
	options := h.listAutopayOptions(ctx, cb.User.ID)
	enabledCount := 0
	for _, option := range options {
		if option.AutoPayEnabled {
			enabledCount++
		}
	}
	text, keyboard := autopayInfoMessage(enabledCount, len(options), h.buildAutopayCancelURL(cb.User.ID))
	if err := h.sender.Edit(ctx, messageRef(cb.ChatID, cb.MessageID), messenger.OutgoingMessage{Text: text, Buttons: keyboard}); err != nil {
		slog.Error("restore autopay info failed", "error", err, "telegram_id", cb.User.ID)
	}
}

func (h *Handler) setAutopayPreference(ctx context.Context, chatID, telegramID int64, enabled bool) {
	if !h.recurringEnabled {
		h.send(ctx, chatID, "Автоплатеж временно недоступен. Подключим его после активации recurring в Robokassa.")
		return
	}
	if enabled {
		h.send(ctx, chatID, "Автоплатеж включается на этапе оплаты через отдельное согласие на автоматические списания. При следующей оплате выберите режим с автоплатежом.")
		return
	}
	if err := h.store.SetUserAutoPayEnabled(ctx, telegramID, enabled, time.Now().UTC()); err != nil {
		slog.Error("save user autopay preference failed", "error", err, "telegram_id", telegramID, "enabled", enabled)
		h.send(ctx, chatID, "Не удалось изменить настройку автоплатежа.")
		return
	}
	h.send(ctx, chatID, "Автоплатеж выключен.")
	h.logAuditEvent(ctx, telegramID, 0, domain.AuditActionAutopayDisabled, "")
}

func (h *Handler) disableAutopayConfirmed(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 || cb.MessageID == 0 {
		return
	}
	if !h.recurringEnabled {
		h.send(ctx, cb.ChatID, "Автоплатеж временно недоступен. Подключим его после активации recurring в Robokassa.")
		return
	}
	if err := h.store.SetUserAutoPayEnabled(ctx, cb.User.ID, false, time.Now().UTC()); err != nil {
		slog.Error("disable autopay failed", "error", err, "telegram_id", cb.User.ID)
		h.send(ctx, cb.ChatID, "Не удалось изменить настройку автоплатежа.")
		return
	}
	if err := h.store.DisableAutoPayForActiveSubscriptions(ctx, cb.User.ID, time.Now().UTC()); err != nil {
		slog.Error("disable subscription autopay failed", "error", err, "telegram_id", cb.User.ID)
		h.send(ctx, cb.ChatID, "Не удалось отключить автоплатеж для активных подписок.")
		return
	}
	text := "🔁 Автоплатеж отключен.\n\nНовые автоматические списания больше не будут выполняться. Доступ сохранится до конца уже оплаченного периода."
	if err := h.sender.Edit(ctx, messageRef(cb.ChatID, cb.MessageID), messenger.OutgoingMessage{Text: text}); err != nil {
		slog.Error("edit autopay disabled message failed", "error", err, "telegram_id", cb.User.ID)
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
	if cb.ChatID == 0 || cb.MessageID == 0 {
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
				Text:   "Отключить: " + option.Name,
				Action: menuCallbackAutopayOffSub + strconv.FormatInt(option.SubscriptionID, 10),
			}})
			continue
		}
		if option.Reactivatable {
			rows = append(rows, []messenger.ActionButton{{
				Text:   "Включить обратно: " + option.Name,
				Action: menuCallbackAutopayOnSub + strconv.FormatInt(option.SubscriptionID, 10),
			}})
			continue
		}
		url := h.buildAutopayCheckoutURL(option.ConnectorID)
		if url == "" {
			continue
		}
		rows = append(rows, []messenger.ActionButton{{
			Text: "Оформить автоплатеж: " + option.Name,
			URL:  url,
		}})
	}
	rows = append(rows, []messenger.ActionButton{{
		Text:   "Назад",
		Action: menuCallbackAutopay,
	}})
	text := "🔁 Выберите подписку\n\nДля каждой подписки доступно свое действие: повторное включение без оплаты или оформление автоплатежа для будущих периодов."
	if err := h.sender.Edit(ctx, messageRef(cb.ChatID, cb.MessageID), messenger.OutgoingMessage{Text: text, Buttons: rows}); err != nil {
		slog.Error("show autopay subscription chooser failed", "error", err, "telegram_id", cb.User.ID)
	}
}

func (h *Handler) reactivateAutopayForSubscription(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 || cb.MessageID == 0 {
		return
	}
	subIDRaw := strings.TrimPrefix(cb.Data, menuCallbackAutopayOnSub)
	subID, err := strconv.ParseInt(subIDRaw, 10, 64)
	if err != nil || subID <= 0 {
		h.send(ctx, cb.ChatID, "Не удалось определить подписку для повторного включения автоплатежа.")
		return
	}
	sub, found, err := h.store.GetSubscriptionByID(ctx, subID)
	if err != nil || !found || sub.TelegramID != cb.User.ID {
		h.send(ctx, cb.ChatID, "Подписка не найдена.")
		return
	}
	if sub.Status != domain.SubscriptionStatusActive {
		h.send(ctx, cb.ChatID, "Автоплатеж можно включить только для активной подписки.")
		return
	}
	if sub.AutoPayEnabled {
		h.restoreAutopayInfo(ctx, cb)
		return
	}
	paymentRow, found, err := h.store.GetPaymentByID(ctx, sub.PaymentID)
	if err != nil || !found || !paymentRow.AutoPayEnabled {
		h.send(ctx, cb.ChatID, "Для этой подписки нельзя включить автоплатеж без новой оплаты.")
		return
	}
	connector, found, err := h.store.GetConnector(ctx, sub.ConnectorID)
	if err != nil || !found {
		h.send(ctx, cb.ChatID, "Тариф не найден.")
		return
	}
	recurringConsent, consentErr := h.buildRecurringConsent(ctx, cb.User.ID, connector)
	if consentErr != nil {
		h.send(ctx, cb.ChatID, "Не удалось подтвердить согласие на автоплатеж.")
		return
	}
	if err := h.store.CreateRecurringConsent(ctx, recurringConsent); err != nil {
		h.send(ctx, cb.ChatID, "Не удалось сохранить согласие на автоплатеж.")
		return
	}
	now := time.Now().UTC()
	if err := h.store.SetSubscriptionAutoPayEnabled(ctx, sub.ID, true, now); err != nil {
		h.send(ctx, cb.ChatID, "Не удалось включить автоплатеж для подписки.")
		return
	}
	h.syncUserAutoPayPreference(ctx, cb.User.ID, now)
	h.logAuditEvent(ctx, cb.User.ID, sub.ConnectorID, domain.AuditActionRecurringConsentGranted, "source=autopay_reactivate")
	h.logAuditEvent(ctx, cb.User.ID, sub.ConnectorID, domain.AuditActionAutopayEnabled, "source=autopay_reactivate;subscription_id="+strconv.FormatInt(sub.ID, 10))
	text := "🔁 Автоплатеж снова включен.\n\nДля этой активной подписки будущие списания снова будут выполняться автоматически. Повторная оплата не потребовалась."
	if err := h.sender.Edit(ctx, messageRef(cb.ChatID, cb.MessageID), messenger.OutgoingMessage{Text: text}); err != nil {
		slog.Error("edit autopay reactivated message failed", "error", err, "telegram_id", cb.User.ID)
	}
}

func (h *Handler) disableAutopayForSubscription(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 || cb.MessageID == 0 {
		return
	}
	subIDRaw := strings.TrimPrefix(cb.Data, menuCallbackAutopayOffSub)
	subID, err := strconv.ParseInt(subIDRaw, 10, 64)
	if err != nil || subID <= 0 {
		h.send(ctx, cb.ChatID, "Не удалось определить подписку для отключения автоплатежа.")
		return
	}
	sub, found, err := h.store.GetSubscriptionByID(ctx, subID)
	if err != nil || !found || sub.TelegramID != cb.User.ID {
		h.send(ctx, cb.ChatID, "Подписка не найдена.")
		return
	}
	if sub.Status != domain.SubscriptionStatusActive {
		h.send(ctx, cb.ChatID, "Автоплатеж можно отключить только для активной подписки.")
		return
	}
	if !sub.AutoPayEnabled {
		h.restoreAutopayInfo(ctx, cb)
		return
	}
	now := time.Now().UTC()
	if err := h.store.SetSubscriptionAutoPayEnabled(ctx, sub.ID, false, now); err != nil {
		h.send(ctx, cb.ChatID, "Не удалось отключить автоплатеж для подписки.")
		return
	}
	h.syncUserAutoPayPreference(ctx, cb.User.ID, now)
	connectorName := ""
	if connector, found, err := h.store.GetConnector(ctx, sub.ConnectorID); err == nil && found {
		connectorName = connector.Name
	}
	h.logAuditEvent(ctx, cb.User.ID, sub.ConnectorID, domain.AuditActionAutopayDisabled, "source=bot_menu;subscription_id="+strconv.FormatInt(sub.ID, 10))
	text := "🔁 Автоплатеж отключен."
	if strings.TrimSpace(connectorName) != "" {
		text = "🔁 Автоплатеж отключен для подписки «" + connectorName + "»."
	}
	text += "\n\nНовые автоматические списания для этого тарифа больше не будут выполняться. Доступ сохранится до конца уже оплаченного периода."
	if err := h.sender.Edit(ctx, messageRef(cb.ChatID, cb.MessageID), messenger.OutgoingMessage{Text: text}); err != nil {
		slog.Error("edit autopay disabled per subscription message failed", "error", err, "telegram_id", cb.User.ID)
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

func autopayInfoMessage(enabledCount, totalCount int, cancelURL string) (string, [][]messenger.ActionButton) {
	text := "🔁 Автоплатеж\n\n"
	if enabledCount > 0 {
		text += fmt.Sprintf("Автоплатеж включен для %d из %d активных подписок.", enabledCount, totalCount)
		rows := make([][]messenger.ActionButton, 0, 2)
		if strings.TrimSpace(cancelURL) != "" {
			text += "\n\nНа публичной странице можно отключить автоплатеж для конкретной подписки."
			rows = append(rows, []messenger.ActionButton{
				buttonURL("Страница отключения", cancelURL),
			})
		}
		rows = append(rows, []messenger.ActionButton{
			buttonAction("Управлять подписками", menuCallbackAutopayPick),
		})
		return text, rows
	}
	text += "Автоплатеж сейчас не включен ни для одной активной подписки."
	if totalCount == 0 {
		return text, nil
	}
	text += "\n\nВыберите подписку, для которой хотите включить или настроить автоплатеж."
	return text, [][]messenger.ActionButton{{
		buttonAction("Выбрать подписку", menuCallbackAutopayPick),
	}}
}

func resolveChannelForBot(channelURL, chatID string) string {
	explicit := strings.TrimSpace(channelURL)
	if explicit != "" {
		if normalized := normalizeTelegramPublicLink(explicit); normalized != "" {
			return normalized
		}
	}
	raw := strings.TrimSpace(chatID)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "@") {
		return "https://t.me/" + strings.TrimPrefix(raw, "@")
	}
	normalized := strings.TrimPrefix(raw, "-")
	normalized = strings.TrimPrefix(normalized, "100")
	if normalized == "" {
		return ""
	}
	if _, err := strconv.ParseInt(normalized, 10, 64); err != nil {
		return ""
	}
	return "https://t.me/c/" + normalized
}

func normalizeTelegramPublicLink(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	v = strings.TrimPrefix(v, "https://")
	v = strings.TrimPrefix(v, "http://")
	v = strings.TrimPrefix(v, "t.me/")
	v = strings.TrimPrefix(v, "telegram.me/")
	v = strings.TrimPrefix(v, "@")
	v = strings.TrimPrefix(v, "/")
	if v == "" || strings.Contains(v, " ") {
		return ""
	}
	return "https://t.me/" + v
}
