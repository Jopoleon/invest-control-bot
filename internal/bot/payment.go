package bot

import (
	"context"
	"crypto/rand"
	"encoding/binary"
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
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgConnectorUnavailable)
		return
	}
	h.logAuditEvent(ctx, cb.User, connectorID, domain.AuditActionPayClicked, "")
	connector, ok, err := h.store.GetConnector(ctx, connectorID)
	if err != nil {
		slog.Error("load connector for pay failed", "error", err, "connector_id", connectorID)
		return
	}
	if !ok || !connector.IsActive {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgConnectorUnavailable)
		return
	}
	if h.payment == nil {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgPaymentProviderNotConfigured)
		return
	}
	if handled := h.sendExistingSubscriptionMessage(ctx, cb.ChatID, cb.User, connectorID); handled {
		return
	}
	user, resolved := h.resolveMessengerUser(ctx, cb.User)
	if !resolved {
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgPaymentProfilePrepareFailed)
		return
	}
	effectiveRecurring := false
	if hasExplicitRecurring {
		effectiveRecurring = h.recurringEnabled && selectedRecurring
	}
	invoiceID := generateInvoiceID()

	checkoutURL, err := h.payment.CreateCheckoutURL(ctx, payment.Request{
		ConnectorID:     connectorID,
		AmountRUB:       connector.PriceRUB,
		InvoiceID:       invoiceID,
		Description:     connector.Name,
		EnableRecurring: effectiveRecurring,
	})
	if err != nil {
		slog.Error("create checkout url failed", "error", err, "connector_id", connectorID, "messenger_kind", cb.User.Kind, "messenger_user_id", cb.User.ID)
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgPaymentLinkFailed)
		return
	}

	if effectiveRecurring {
		recurringConsent, consentErr := h.buildRecurringConsent(ctx, user.ID, connector)
		if consentErr != nil {
			slog.Error("build recurring consent failed", "error", consentErr, "connector_id", connectorID, "messenger_kind", cb.User.Kind, "messenger_user_id", cb.User.ID)
			h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopayDocsMissing)
			return
		}
		if err := h.store.CreateRecurringConsent(ctx, recurringConsent); err != nil {
			slog.Error("save recurring consent failed", "error", err, "connector_id", connectorID, "messenger_kind", cb.User.Kind, "messenger_user_id", cb.User.ID)
			h.sendTo(ctx, cb.ChatID, cb.User, botMsgAutopayConsentSaveFailed)
			return
		}
		h.logAuditEvent(ctx, cb.User, connectorID, domain.AuditActionRecurringConsentGranted, "")
	}

	token := invoiceID
	err = h.store.CreatePayment(ctx, domain.Payment{
		Provider:       h.payment.ProviderName(),
		Status:         domain.PaymentStatusPending,
		Token:          token,
		UserID:         user.ID,
		ConnectorID:    connectorID,
		AmountRUB:      connector.PriceRUB,
		AutoPayEnabled: effectiveRecurring,
		CheckoutURL:    checkoutURL,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	})
	if err != nil {
		slog.Error("save payment failed", "error", err, "connector_id", connectorID, "messenger_kind", cb.User.Kind, "messenger_user_id", cb.User.ID)
		h.sendTo(ctx, cb.ChatID, cb.User, botMsgPaymentLinkFailed)
		return
	}
	h.logAuditEvent(ctx, cb.User, connectorID, domain.AuditActionPaymentCreated, "token="+token)

	out := messenger.OutgoingMessage{
		Text: botPaymentLinkCreated(h.payment.ProviderName()),
		Buttons: [][]messenger.ActionButton{{
			buttonURL(botBtnGoToPayment, checkoutURL),
		}},
	}
	if err := h.sender.Send(ctx, recipientRef(cb.ChatID, cb.User), out); err != nil {
		slog.Error("send pay link failed", "error", err, "connector_id", connectorID, "messenger_kind", cb.User.Kind, "messenger_user_id", cb.User.ID)
		return
	}
	details := h.payment.ProviderName()
	if effectiveRecurring {
		details += ";autopay=on"
	}
	h.logAuditEvent(ctx, cb.User, connectorID, domain.AuditActionPayLinkSent, details)
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
