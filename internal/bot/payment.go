package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Jopoleon/telega-bot-fedor/internal/payment"
	"github.com/go-telegram/bot/models"
)

// handlePay creates checkout URL with currently selected payment provider.
func (h *Handler) handlePay(ctx context.Context, cb *models.CallbackQuery) {
	connectorID := strings.TrimPrefix(cb.Data, "pay:")
	h.logAuditEvent(ctx, cb.From.ID, connectorID, "pay_clicked", "")
	connector, ok, err := h.store.GetConnector(ctx, connectorID)
	if err != nil {
		slog.Error("load connector for pay failed", "error", err, "connector_id", connectorID)
		return
	}
	if !ok || !connector.IsActive {
		h.send(ctx, cb.From.ID, "Коннектор не найден или отключен.")
		return
	}
	if h.payment == nil {
		h.send(ctx, cb.From.ID, "Платежный провайдер пока не настроен.")
		return
	}

	checkoutURL, err := h.payment.CreateCheckoutURL(ctx, payment.Request{
		UserTelegramID: cb.From.ID,
		ConnectorID:    connectorID,
		AmountRUB:      connector.PriceRUB,
	})
	if err != nil {
		slog.Error("create checkout url failed", "error", err, "connector_id", connectorID, "telegram_id", cb.From.ID)
		h.send(ctx, cb.From.ID, "Не удалось сформировать ссылку оплаты. Попробуйте позже.")
		return
	}

	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: "Перейти к оплате", URL: checkoutURL},
	}}}
	if err := h.tg.SendMessage(ctx, cb.From.ID,
		fmt.Sprintf("Сформирована ссылка оплаты через %s в тестовом режиме.", h.payment.ProviderName()),
		keyboard,
	); err != nil {
		slog.Error("send pay link failed", "error", err, "connector_id", connectorID, "telegram_id", cb.From.ID)
		return
	}
	h.logAuditEvent(ctx, cb.From.ID, connectorID, "pay_link_sent", h.payment.ProviderName())
}
