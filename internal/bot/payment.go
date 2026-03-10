package bot

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Jopoleon/telega-bot-fedor/internal/payment"
	"github.com/go-telegram/bot/models"
)

// handlePay creates checkout URL with currently selected payment provider.
func (h *Handler) handlePay(ctx context.Context, cb *models.CallbackQuery) {
	connectorID := strings.TrimPrefix(cb.Data, "pay:")
	connector, ok, err := h.store.GetConnector(ctx, connectorID)
	if err != nil {
		log.Printf("load connector for pay failed: %v", err)
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
		log.Printf("create checkout url failed: %v", err)
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
		log.Printf("send pay link failed: %v", err)
	}
}
