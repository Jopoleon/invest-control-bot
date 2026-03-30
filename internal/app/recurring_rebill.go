package app

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/payment"
)

var errRebillRequestFailed = errors.New("rebill request failed")

func (a *application) triggerRebill(ctx context.Context, subscriptionID int64, source string) (rebillResponse, error) {
	if a.robokassaService == nil {
		return rebillResponse{}, errors.New("rebill provider is not configured")
	}

	subscription, found, err := a.store.GetSubscriptionByID(ctx, subscriptionID)
	if err != nil {
		return rebillResponse{}, err
	}
	if !found {
		return rebillResponse{}, errors.New("subscription not found")
	}
	if subscription.Status != domain.SubscriptionStatusActive {
		return rebillResponse{}, errors.New("subscription is not active")
	}
	if !subscription.AutoPayEnabled {
		return rebillResponse{}, errors.New("autopay is disabled for subscription")
	}
	if pending, ok, err := a.store.GetPendingRebillPaymentBySubscription(ctx, subscription.ID); err != nil {
		return rebillResponse{}, err
	} else if ok {
		return rebillResponse{OK: true, InvoiceID: pending.Token, Existing: true}, nil
	}

	parentPayment, found, err := a.store.GetPaymentByID(ctx, subscription.PaymentID)
	if err != nil {
		return rebillResponse{}, err
	}
	if !found || strings.TrimSpace(parentPayment.Token) == "" {
		return rebillResponse{}, errors.New("parent payment is missing token")
	}

	connector, found, err := a.store.GetConnector(ctx, subscription.ConnectorID)
	if err != nil {
		return rebillResponse{}, err
	}
	if !found {
		return rebillResponse{}, errors.New("connector not found")
	}

	// The generated invoiceID becomes the next merchant-side Robokassa InvoiceID
	// and is persisted in payments.token for callbacks and later reconciliation.
	invoiceID := generateInvoiceID()
	now := time.Now().UTC()
	pendingPayment := domain.Payment{
		Provider:          "robokassa",
		ProviderPaymentID: "rebill_parent:" + parentPayment.Token,
		Status:            domain.PaymentStatusPending,
		Token:             invoiceID,
		UserID:            subscription.UserID,
		ConnectorID:       subscription.ConnectorID,
		SubscriptionID:    subscription.ID,
		ParentPaymentID:   parentPayment.ID,
		AmountRUB:         connector.PriceRUB,
		AutoPayEnabled:    true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := a.store.CreatePayment(ctx, pendingPayment); err != nil {
		if existing, ok, lookupErr := a.store.GetPendingRebillPaymentBySubscription(ctx, subscription.ID); lookupErr == nil && ok {
			return rebillResponse{OK: true, InvoiceID: existing.Token, Existing: true}, nil
		}
		return rebillResponse{}, err
	}

	createdPayment, found, err := a.store.GetPaymentByToken(ctx, invoiceID)
	if err != nil {
		return rebillResponse{}, err
	}
	if !found {
		return rebillResponse{}, errors.New("created rebill payment not found")
	}
	pendingPayment = createdPayment

	if err := a.robokassaService.CreateRebill(ctx, payment.RebillRequest{
		InvoiceID:         invoiceID,
		PreviousInvoiceID: parentPayment.Token,
		AmountRUB:         connector.PriceRUB,
		Description:       connector.Name,
	}); err != nil {
		if _, markErr := a.store.UpdatePaymentFailed(ctx, pendingPayment.ID, "rebill_request_failed:"+parentPayment.Token, time.Now().UTC()); markErr != nil {
			logStoreError("mark rebill payment failed failed", markErr, "payment_id", pendingPayment.ID)
		}
		_ = a.store.SaveAuditEvent(ctx, a.buildAppTargetAuditEvent(
			ctx,
			subscription.UserID,
			"",
			subscription.ConnectorID,
			domain.AuditActionRebillRequestFailed,
			"subscription_id="+strconv.FormatInt(subscription.ID, 10)+";invoice_id="+invoiceID+";source="+source+";error="+err.Error(),
			time.Now().UTC(),
		))
		return rebillResponse{}, errRebillRequestFailed
	}

	if err := a.store.SaveAuditEvent(ctx, a.buildAppTargetAuditEvent(
		ctx,
		subscription.UserID,
		"",
		subscription.ConnectorID,
		domain.AuditActionRebillRequested,
		"subscription_id="+strconv.FormatInt(subscription.ID, 10)+";invoice_id="+invoiceID+";parent="+parentPayment.Token+";source="+source,
		now,
	)); err != nil {
		logAuditError(domain.AuditActionRebillRequested, err)
	}

	return rebillResponse{OK: true, InvoiceID: invoiceID}, nil
}

func (a *application) shouldTriggerScheduledRebill(ctx context.Context, sub domain.Subscription, now time.Time) (bool, error) {
	targetAttempt := recurringAttemptOrdinal(now, sub.EndsAt)
	if targetAttempt == 0 {
		return false, nil
	}

	payments, err := a.store.ListPayments(ctx, domain.PaymentListQuery{
		UserID: sub.UserID,
		Limit:  500,
	})
	if err != nil {
		return false, err
	}

	attempts := 0
	for _, p := range payments {
		if p.SubscriptionID != sub.ID || p.ParentPaymentID <= 0 {
			continue
		}
		switch p.Status {
		case domain.PaymentStatusPending, domain.PaymentStatusPaid:
			return false, nil
		case domain.PaymentStatusFailed:
			attempts++
		default:
			attempts++
		}
	}

	return attempts < targetAttempt, nil
}

func recurringAttemptOrdinal(now, endsAt time.Time) int {
	remaining := endsAt.Sub(now)
	switch {
	case remaining <= 0:
		return 0
	case remaining <= 24*time.Hour:
		return 3
	case remaining <= 48*time.Hour:
		return 2
	case remaining <= 72*time.Hour:
		return 1
	default:
		return 0
	}
}
