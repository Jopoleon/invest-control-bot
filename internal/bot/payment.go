package bot

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log/slog"
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
	autoPayEnabled, _, err := h.store.GetUserAutoPayEnabled(ctx, cb.From.ID)
	if err != nil {
		slog.Error("load user autopay preference failed", "error", err, "telegram_id", cb.From.ID)
	}
	invoiceID := generateInvoiceID()

	checkoutURL, err := h.payment.CreateCheckoutURL(ctx, payment.Request{
		UserTelegramID:  cb.From.ID,
		ConnectorID:     connectorID,
		AmountRUB:       connector.PriceRUB,
		InvoiceID:       invoiceID,
		Description:     connector.Name,
		EnableRecurring: autoPayEnabled,
	})
	if err != nil {
		slog.Error("create checkout url failed", "error", err, "connector_id", connectorID, "telegram_id", cb.From.ID)
		h.send(ctx, cb.From.ID, "Не удалось сформировать ссылку оплаты. Попробуйте позже.")
		return
	}
	token := invoiceID
	err = h.store.CreatePayment(ctx, domain.Payment{
		Provider:       h.payment.ProviderName(),
		Status:         domain.PaymentStatusPending,
		Token:          token,
		TelegramID:     cb.From.ID,
		ConnectorID:    connectorID,
		AmountRUB:      connector.PriceRUB,
		AutoPayEnabled: autoPayEnabled,
		CheckoutURL:    checkoutURL,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
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
	details := h.payment.ProviderName()
	if autoPayEnabled {
		details += ";autopay=on"
	}
	h.logAuditEvent(ctx, cb.From.ID, connectorID, "pay_link_sent", details)
}

func generateInvoiceID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	v := int64(binary.BigEndian.Uint64(b[:]) & 0x7fffffffffffffff)
	if v < 1_000_000_000 {
		v += 1_000_000_000
	}
	return strconv.FormatInt(v, 10)
}
