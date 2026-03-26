package bot

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/payment"
)

// handlePay creates checkout URL with currently selected payment provider.
func (h *Handler) handlePay(ctx context.Context, cb messenger.IncomingAction) {
	connectorID, selectedRecurring, hasExplicitRecurring, ok := parsePayCallbackData(cb.Data)
	if !ok {
		h.send(ctx, cb.ChatID, "Коннектор не найден или отключен.")
		return
	}
	h.logAuditEvent(ctx, cb.User.ID, connectorID, domain.AuditActionPayClicked, "")
	connector, ok, err := h.store.GetConnector(ctx, connectorID)
	if err != nil {
		slog.Error("load connector for pay failed", "error", err, "connector_id", connectorID)
		return
	}
	if !ok || !connector.IsActive {
		h.send(ctx, cb.ChatID, "Коннектор не найден или отключен.")
		return
	}
	if h.payment == nil {
		h.send(ctx, cb.ChatID, "Платежный провайдер пока не настроен.")
		return
	}
	if handled := h.sendExistingSubscriptionMessage(ctx, cb.ChatID, cb.User.ID, connectorID); handled {
		return
	}
	user, resolved := h.resolveTelegramUser(ctx, cb.User.ID, cb.User.Username)
	if !resolved {
		h.send(ctx, cb.ChatID, "Не удалось подготовить профиль пользователя для оплаты. Попробуйте позже.")
		return
	}
	autoPayEnabled, _, err := h.store.GetUserAutoPayEnabled(ctx, cb.User.ID)
	if err != nil {
		slog.Error("load user autopay preference failed", "error", err, "telegram_id", cb.User.ID)
	}
	effectiveRecurring := h.recurringEnabled && autoPayEnabled
	if hasExplicitRecurring {
		effectiveRecurring = h.recurringEnabled && selectedRecurring
	}
	invoiceID := generateInvoiceID()

	checkoutURL, err := h.payment.CreateCheckoutURL(ctx, payment.Request{
		UserTelegramID:  cb.User.ID,
		ConnectorID:     connectorID,
		AmountRUB:       connector.PriceRUB,
		InvoiceID:       invoiceID,
		Description:     connector.Name,
		EnableRecurring: effectiveRecurring,
	})
	if err != nil {
		slog.Error("create checkout url failed", "error", err, "connector_id", connectorID, "telegram_id", cb.User.ID)
		h.send(ctx, cb.ChatID, "Не удалось сформировать ссылку оплаты. Попробуйте позже.")
		return
	}

	if effectiveRecurring {
		recurringConsent, consentErr := h.buildRecurringConsent(ctx, cb.User.ID, connector)
		if consentErr != nil {
			slog.Error("build recurring consent failed", "error", consentErr, "connector_id", connectorID, "telegram_id", cb.User.ID)
			h.send(ctx, cb.ChatID, "Автоплатеж пока недоступен: не найдены обязательные документы для согласия.")
			return
		}
		if err := h.store.CreateRecurringConsent(ctx, recurringConsent); err != nil {
			slog.Error("save recurring consent failed", "error", err, "connector_id", connectorID, "telegram_id", cb.User.ID)
			h.send(ctx, cb.ChatID, "Не удалось сохранить согласие на автоплатеж. Попробуйте еще раз.")
			return
		}
		h.logAuditEvent(ctx, cb.User.ID, connectorID, domain.AuditActionRecurringConsentGranted, "")
		if err := h.store.SetUserAutoPayEnabled(ctx, cb.User.ID, true, time.Now().UTC()); err != nil {
			slog.Error("save user autopay preference after consent failed", "error", err, "telegram_id", cb.User.ID)
		}
	}

	token := invoiceID
	err = h.store.CreatePayment(ctx, domain.Payment{
		Provider:       h.payment.ProviderName(),
		Status:         domain.PaymentStatusPending,
		Token:          token,
		UserID:         user.ID,
		TelegramID:     cb.User.ID,
		ConnectorID:    connectorID,
		AmountRUB:      connector.PriceRUB,
		AutoPayEnabled: effectiveRecurring,
		CheckoutURL:    checkoutURL,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	})
	if err != nil {
		slog.Error("save payment failed", "error", err, "connector_id", connectorID, "telegram_id", cb.User.ID)
		h.send(ctx, cb.ChatID, "Не удалось сформировать ссылку оплаты. Попробуйте позже.")
		return
	}
	h.logAuditEvent(ctx, cb.User.ID, connectorID, domain.AuditActionPaymentCreated, "token="+token)

	out := messenger.OutgoingMessage{
		Text: fmt.Sprintf("Сформирована ссылка оплаты через %s в тестовом режиме.", h.payment.ProviderName()),
		Buttons: [][]messenger.ActionButton{{
			buttonURL("Перейти к оплате", checkoutURL),
		}},
	}
	if err := h.sender.Send(ctx, chatRef(cb.ChatID), out); err != nil {
		slog.Error("send pay link failed", "error", err, "connector_id", connectorID, "telegram_id", cb.User.ID)
		return
	}
	details := h.payment.ProviderName()
	if effectiveRecurring {
		details += ";autopay=on"
	}
	h.logAuditEvent(ctx, cb.User.ID, connectorID, domain.AuditActionPayLinkSent, details)
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
