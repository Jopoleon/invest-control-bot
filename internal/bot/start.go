package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-telegram/bot/models"
)

// handleStart resolves connector by payload and shows tariff card with consent button.
func (h *Handler) handleStart(ctx context.Context, msg *models.Message) {
	parts := strings.Fields(strings.TrimSpace(msg.Text))
	if len(parts) < 2 {
		h.send(ctx, msg.Chat.ID, "Нужна ссылка вида /start <connector_payload>.")
		return
	}

	payload := strings.TrimSpace(parts[1])
	connector, ok, err := h.store.GetConnectorByStartPayload(ctx, payload)
	if err != nil {
		slog.Error("load connector by payload failed", "error", err, "payload", payload)
		return
	}
	if !ok {
		connector, ok, err = h.store.GetConnector(ctx, payload)
		if err != nil {
			slog.Error("fallback load connector by id failed", "error", err, "connector_id", payload)
			return
		}
	}
	if !ok || !connector.IsActive {
		h.send(ctx, msg.Chat.ID, "Коннектор не найден или отключен.")
		return
	}
	h.logAuditEvent(ctx, msg.From.ID, connector.ID, "start_opened", "payload="+payload)

	offerURL := connector.OfferURL
	if offerURL == "" {
		offerURL = "https://example.com/contract"
	}
	privacyURL := connector.PrivacyURL
	if privacyURL == "" {
		privacyURL = "https://example.com/policy"
	}

	text := fmt.Sprintf(
		"%s\n%s\n⚡️ Подписка: %d ₽\nПериод оплаты: Ежемесячно\nЧтобы продолжить, вам необходимо принять условия публичной оферты (%s) и политики обработки персональных данных (%s).",
		connector.Name,
		connector.Description,
		connector.PriceRUB,
		offerURL,
		privacyURL,
	)

	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: "Принимаю условия", CallbackData: "accept_terms:" + connector.ID},
	}}}

	if err := h.tg.SendMessage(ctx, msg.Chat.ID, text, keyboard); err != nil {
		slog.Error("send start message failed", "error", err, "chat_id", msg.Chat.ID, "connector_id", connector.ID)
	}
}
