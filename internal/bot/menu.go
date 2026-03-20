package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/go-telegram/bot/models"
)

const (
	menuCallbackPrefix       = "menu:"
	menuCallbackSubscription = "menu:subscription"
	menuCallbackPayments     = "menu:payments"
	menuCallbackAutopay      = "menu:autopay"
	menuCallbackAutopayOn    = "menu:autopay:on"
	menuCallbackAutopayOff   = "menu:autopay:off"
)

func (h *Handler) sendMenu(ctx context.Context, chatID int64) {
	rows := [][]models.InlineKeyboardButton{
		{
			{Text: "📄 Моя подписка", CallbackData: menuCallbackSubscription},
		},
		{
			{Text: "💳 Платежи", CallbackData: menuCallbackPayments},
		},
	}
	if h.recurringEnabled {
		rows = append(rows, []models.InlineKeyboardButton{
			{Text: "🔁 Автоплатеж", CallbackData: menuCallbackAutopay},
		})
	}
	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: rows}
	if err := h.tg.SendMessage(ctx, chatID, "Личный кабинет:", keyboard); err != nil {
		slog.Error("send menu failed", "error", err, "chat_id", chatID)
	}
}

func (h *Handler) handleMenuCallback(ctx context.Context, cb *models.CallbackQuery) {
	if cb == nil || cb.Message.Message == nil {
		return
	}
	chatID := cb.Message.Message.Chat.ID

	switch cb.Data {
	case menuCallbackSubscription:
		h.sendSubscriptionOverview(ctx, chatID, cb.From.ID)
	case menuCallbackPayments:
		h.sendPaymentHistory(ctx, chatID, cb.From.ID)
	case menuCallbackAutopay:
		h.sendAutopayInfo(ctx, chatID, cb.From.ID)
	case menuCallbackAutopayOn:
		h.setAutopayPreference(ctx, chatID, cb.From.ID, true)
	case menuCallbackAutopayOff:
		h.setAutopayPreference(ctx, chatID, cb.From.ID, false)
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
	enabled, _, err := h.store.GetUserAutoPayEnabled(ctx, telegramID)
	if err != nil {
		slog.Error("load user autopay preference failed", "error", err, "telegram_id", telegramID)
		h.send(ctx, chatID, "Не удалось загрузить настройки автоплатежа.")
		return
	}
	status := "выключен"
	actionLabel := "Включить автоплатеж"
	actionCallback := menuCallbackAutopayOn
	if enabled {
		status = "включен"
		actionLabel = "Выключить автоплатеж"
		actionCallback = menuCallbackAutopayOff
	}
	text := "🔁 Автоплатеж\n\nТекущий статус: " + status + ".\nИзменение применяется к следующим платежам через Robokassa recurring."
	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
		{
			{Text: actionLabel, CallbackData: actionCallback},
		},
	}}
	if err := h.tg.SendMessage(ctx, chatID, text, keyboard); err != nil {
		slog.Error("send autopay info failed", "error", err, "telegram_id", telegramID)
		return
	}
	h.logAuditEvent(ctx, telegramID, 0, domain.AuditActionMenuAutoPayOpened, "")
}

func (h *Handler) setAutopayPreference(ctx context.Context, chatID, telegramID int64, enabled bool) {
	if !h.recurringEnabled {
		h.send(ctx, chatID, "Автоплатеж временно недоступен. Подключим его после активации recurring в Robokassa.")
		return
	}
	if err := h.store.SetUserAutoPayEnabled(ctx, telegramID, enabled, time.Now().UTC()); err != nil {
		slog.Error("save user autopay preference failed", "error", err, "telegram_id", telegramID, "enabled", enabled)
		h.send(ctx, chatID, "Не удалось изменить настройку автоплатежа.")
		return
	}
	if enabled {
		h.send(ctx, chatID, "Автоплатеж включен. Следующая оплата будет создана в recurring-режиме.")
		h.logAuditEvent(ctx, telegramID, 0, domain.AuditActionAutopayEnabled, "")
		return
	}
	h.send(ctx, chatID, "Автоплатеж выключен.")
	h.logAuditEvent(ctx, telegramID, 0, domain.AuditActionAutopayDisabled, "")
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
