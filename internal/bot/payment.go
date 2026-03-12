package bot

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/Jopoleon/telega-bot-fedor/internal/payment"
	"github.com/go-telegram/bot/models"
)

// handlePay creates checkout URL with currently selected payment provider.
func (h *Handler) handlePay(ctx context.Context, cb *models.CallbackQuery) {
	connectorIDRaw := strings.TrimPrefix(cb.Data, "pay:")
	connectorID, err := strconv.ParseInt(connectorIDRaw, 10, 64)
	if err != nil || connectorID <= 0 {
		h.send(ctx, cb.From.ID, "Коннектор не найден или отключен.")
		return
	}
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
	token := paymentTokenFromURL(checkoutURL)
	if token == "" {
		slog.Error("checkout url does not contain payment token", "connector_id", connectorID, "telegram_id", cb.From.ID)
		h.send(ctx, cb.From.ID, "Не удалось сформировать ссылку оплаты. Попробуйте позже.")
		return
	}
	err = h.store.CreatePayment(ctx, domain.Payment{
		Provider:    h.payment.ProviderName(),
		Status:      domain.PaymentStatusPending,
		Token:       token,
		TelegramID:  cb.From.ID,
		ConnectorID: connectorID,
		AmountRUB:   connector.PriceRUB,
		CheckoutURL: checkoutURL,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	})
	if err != nil {
		slog.Error("save payment failed", "error", err, "connector_id", connectorID, "telegram_id", cb.From.ID)
		h.send(ctx, cb.From.ID, "Не удалось сформировать ссылку оплаты. Попробуйте позже.")
		return
	}
	h.logAuditEvent(ctx, cb.From.ID, connectorID, "payment_created", "token="+token)

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

func paymentTokenFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(u.Query().Get("token"))
}
