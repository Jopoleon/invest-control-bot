package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestShouldTriggerScheduledRebill_FirstAttemptWindow(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-recurring-window-1")
	parentInv := "rec-parent-1"

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          parentInv,
		UserID:         seedTelegramUser(t, ctx, st, 990001),
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	})
	parentPayment, found, err := st.GetPaymentByToken(ctx, parentInv)
	if err != nil || !found {
		t.Fatalf("parent payment not found: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         parentPayment.UserID,
		ConnectorID:    parentPayment.ConnectorID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       time.Now().UTC().Add(-24 * time.Hour),
		EndsAt:         time.Now().UTC().Add(70 * time.Hour),
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	subs, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{UserID: parentPayment.UserID, Limit: 10})
	if err != nil || len(subs) != 1 {
		t.Fatalf("subscriptions len=%d err=%v", len(subs), err)
	}

	appCtx := testApplicationForRecurring(t, st)
	ok, err := appCtx.shouldTriggerScheduledRebill(ctx, subs[0], time.Now().UTC())
	if err != nil {
		t.Fatalf("shouldTriggerScheduledRebill err=%v", err)
	}
	if !ok {
		t.Fatalf("shouldTriggerScheduledRebill = false, want true")
	}
}

func TestShouldTriggerScheduledRebill_SkipsWhenPendingExists(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-recurring-window-pending")
	parentInv := "rec-parent-2"

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          parentInv,
		UserID:         seedTelegramUser(t, ctx, st, 990002),
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	})
	parentPayment, found, err := st.GetPaymentByToken(ctx, parentInv)
	if err != nil || !found {
		t.Fatalf("parent payment not found: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         parentPayment.UserID,
		ConnectorID:    parentPayment.ConnectorID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       time.Now().UTC().Add(-24 * time.Hour),
		EndsAt:         time.Now().UTC().Add(20 * time.Hour),
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	subs, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{UserID: parentPayment.UserID, Limit: 10})
	if err != nil || len(subs) != 1 {
		t.Fatalf("subscriptions len=%d err=%v", len(subs), err)
	}

	seedPayment(t, ctx, st, domain.Payment{
		Provider:          "robokassa",
		ProviderPaymentID: "rebill_parent:" + parentInv,
		Status:            domain.PaymentStatusPending,
		Token:             "rebill-pending-1",
		UserID:            parentPayment.UserID,
		ConnectorID:       connectorID,
		SubscriptionID:    subs[0].ID,
		ParentPaymentID:   parentPayment.ID,
		AmountRUB:         2322,
		AutoPayEnabled:    true,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	})

	appCtx := testApplicationForRecurring(t, st)
	ok, err := appCtx.shouldTriggerScheduledRebill(ctx, subs[0], time.Now().UTC())
	if err != nil {
		t.Fatalf("shouldTriggerScheduledRebill err=%v", err)
	}
	if ok {
		t.Fatalf("shouldTriggerScheduledRebill = true, want false")
	}
}

func TestShouldTriggerScheduledRebill_SecondAttemptWindowAfterOneFailure(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-recurring-window-2")
	parentInv := "rec-parent-3"

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          parentInv,
		UserID:         seedTelegramUser(t, ctx, st, 990003),
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	})
	parentPayment, found, err := st.GetPaymentByToken(ctx, parentInv)
	if err != nil || !found {
		t.Fatalf("parent payment not found: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         parentPayment.UserID,
		ConnectorID:    parentPayment.ConnectorID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       time.Now().UTC().Add(-24 * time.Hour),
		EndsAt:         time.Now().UTC().Add(40 * time.Hour),
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	subs, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{UserID: parentPayment.UserID, Limit: 10})
	if err != nil || len(subs) != 1 {
		t.Fatalf("subscriptions len=%d err=%v", len(subs), err)
	}

	seedPayment(t, ctx, st, domain.Payment{
		Provider:          "robokassa",
		ProviderPaymentID: "rebill_parent:" + parentInv,
		Status:            domain.PaymentStatusFailed,
		Token:             "rebill-failed-1",
		UserID:            parentPayment.UserID,
		ConnectorID:       connectorID,
		SubscriptionID:    subs[0].ID,
		ParentPaymentID:   parentPayment.ID,
		AmountRUB:         2322,
		AutoPayEnabled:    true,
		CreatedAt:         time.Now().UTC().Add(-2 * time.Hour),
		UpdatedAt:         time.Now().UTC().Add(-2 * time.Hour),
	})

	appCtx := testApplicationForRecurring(t, st)
	ok, err := appCtx.shouldTriggerScheduledRebill(ctx, subs[0], time.Now().UTC())
	if err != nil {
		t.Fatalf("shouldTriggerScheduledRebill err=%v", err)
	}
	if !ok {
		t.Fatalf("shouldTriggerScheduledRebill = false, want true")
	}
}

func TestShouldTriggerScheduledRebill_ThirdAttemptWindowAfterTwoFailures(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-recurring-window-3")
	parentInv := "rec-parent-4"

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          parentInv,
		UserID:         seedTelegramUser(t, ctx, st, 990004),
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	})
	parentPayment, found, err := st.GetPaymentByToken(ctx, parentInv)
	if err != nil || !found {
		t.Fatalf("parent payment not found: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         parentPayment.UserID,
		ConnectorID:    parentPayment.ConnectorID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       time.Now().UTC().Add(-24 * time.Hour),
		EndsAt:         time.Now().UTC().Add(10 * time.Hour),
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	subs, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{UserID: parentPayment.UserID, Limit: 10})
	if err != nil || len(subs) != 1 {
		t.Fatalf("subscriptions len=%d err=%v", len(subs), err)
	}

	for idx := 1; idx <= 2; idx++ {
		seedPayment(t, ctx, st, domain.Payment{
			Provider:          "robokassa",
			ProviderPaymentID: "rebill_parent:" + parentInv,
			Status:            domain.PaymentStatusFailed,
			Token:             "rebill-failed-third-" + string(rune('0'+idx)),
			UserID:            parentPayment.UserID,
			ConnectorID:       connectorID,
			SubscriptionID:    subs[0].ID,
			ParentPaymentID:   parentPayment.ID,
			AmountRUB:         2322,
			AutoPayEnabled:    true,
			CreatedAt:         time.Now().UTC().Add(time.Duration(-idx) * time.Hour),
			UpdatedAt:         time.Now().UTC().Add(time.Duration(-idx) * time.Hour),
		})
	}

	appCtx := testApplicationForRecurring(t, st)
	ok, err := appCtx.shouldTriggerScheduledRebill(ctx, subs[0], time.Now().UTC())
	if err != nil {
		t.Fatalf("shouldTriggerScheduledRebill err=%v", err)
	}
	if !ok {
		t.Fatalf("shouldTriggerScheduledRebill = false, want true")
	}
}

func TestShouldTriggerScheduledRebill_DoesNotTriggerAfterThreeFailures(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-recurring-window-stop")
	parentInv := "rec-parent-5"

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          parentInv,
		UserID:         seedTelegramUser(t, ctx, st, 990005),
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	})
	parentPayment, found, err := st.GetPaymentByToken(ctx, parentInv)
	if err != nil || !found {
		t.Fatalf("parent payment not found: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         parentPayment.UserID,
		ConnectorID:    parentPayment.ConnectorID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       time.Now().UTC().Add(-24 * time.Hour),
		EndsAt:         time.Now().UTC().Add(6 * time.Hour),
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	subs, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{UserID: parentPayment.UserID, Limit: 10})
	if err != nil || len(subs) != 1 {
		t.Fatalf("subscriptions len=%d err=%v", len(subs), err)
	}

	for idx := 1; idx <= 3; idx++ {
		seedPayment(t, ctx, st, domain.Payment{
			Provider:          "robokassa",
			ProviderPaymentID: "rebill_parent:" + parentInv,
			Status:            domain.PaymentStatusFailed,
			Token:             "rebill-failed-stop-" + string(rune('0'+idx)),
			UserID:            parentPayment.UserID,
			ConnectorID:       connectorID,
			SubscriptionID:    subs[0].ID,
			ParentPaymentID:   parentPayment.ID,
			AmountRUB:         2322,
			AutoPayEnabled:    true,
			CreatedAt:         time.Now().UTC().Add(time.Duration(-idx) * time.Hour),
			UpdatedAt:         time.Now().UTC().Add(time.Duration(-idx) * time.Hour),
		})
	}

	appCtx := testApplicationForRecurring(t, st)
	ok, err := appCtx.shouldTriggerScheduledRebill(ctx, subs[0], time.Now().UTC())
	if err != nil {
		t.Fatalf("shouldTriggerScheduledRebill err=%v", err)
	}
	if ok {
		t.Fatalf("shouldTriggerScheduledRebill = true, want false")
	}
}

func TestProcessRecurringRebills_TriggersScheduledAttempt(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-recurring-process")
	parentInv := "rec-parent-6"

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          parentInv,
		UserID:         seedTelegramUser(t, ctx, st, 990006),
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	})
	parentPayment, found, err := st.GetPaymentByToken(ctx, parentInv)
	if err != nil || !found {
		t.Fatalf("parent payment not found: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         parentPayment.UserID,
		ConnectorID:    parentPayment.ConnectorID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       time.Now().UTC().Add(-24 * time.Hour),
		EndsAt:         time.Now().UTC().Add(70 * time.Hour),
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	rebillCalls := 0
	rebillMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rebillCalls++
		_, _ = w.Write([]byte("OK+scheduled"))
	}))
	defer rebillMock.Close()

	cfg := testRecurringConfig(rebillMock.URL)
	appCtx, err := newApplication(cfg, st, appInitOptions{ensureTelegramSetup: true})
	if err != nil {
		t.Fatalf("newApplication: %v", err)
	}

	processRecurringRebills(ctx, appCtx)

	if rebillCalls != 1 {
		t.Fatalf("rebill provider calls=%d want=1", rebillCalls)
	}
	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{UserID: parentPayment.UserID, Limit: 20})
	if err != nil {
		t.Fatalf("list payments: %v", err)
	}
	pendingCount := 0
	for _, p := range payments {
		if p.SubscriptionID > 0 && p.ParentPaymentID == parentPayment.ID && p.Status == domain.PaymentStatusPending {
			pendingCount++
		}
	}
	if pendingCount != 1 {
		t.Fatalf("pending rebill payments=%d want=1", pendingCount)
	}
}

func TestProcessRecurringRebills_EndToEndCallbackActivatesExactlyOneRenewal(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-recurring-end-to-end")
	parentInv := "rec-parent-e2e"
	userID := seedTelegramUser(t, ctx, st, 990777)
	now := time.Now().UTC()

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          parentInv,
		UserID:         userID,
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      now.Add(-24 * time.Hour),
		UpdatedAt:      now.Add(-24 * time.Hour),
		PaidAt:         ptrTime(now.Add(-24 * time.Hour)),
	})
	parentPayment, found, err := st.GetPaymentByToken(ctx, parentInv)
	if err != nil || !found {
		t.Fatalf("parent payment not found: found=%v err=%v", found, err)
	}
	parentStart := now.Add(-24 * time.Hour).Truncate(time.Second)
	parentEnd := now.Add(70 * time.Hour).Truncate(time.Second)
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         userID,
		ConnectorID:    connectorID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       parentStart,
		EndsAt:         parentEnd,
		CreatedAt:      parentStart,
		UpdatedAt:      parentStart,
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	rebillMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK+scheduled"))
	}))
	defer rebillMock.Close()

	cfg := testRecurringConfig(rebillMock.URL)
	appCtx, err := newApplication(cfg, st, appInitOptions{ensureTelegramSetup: true})
	if err != nil {
		t.Fatalf("newApplication: %v", err)
	}
	processRecurringRebills(ctx, appCtx)

	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{UserID: userID, Limit: 20})
	if err != nil {
		t.Fatalf("list payments: %v", err)
	}
	var child domain.Payment
	found = false
	for _, paymentRow := range payments {
		if paymentRow.ParentPaymentID == parentPayment.ID && paymentRow.Status == domain.PaymentStatusPending {
			child = paymentRow
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("pending child payment not found")
	}

	handler := testServerHandler(t, st, "test-pass2")
	code, body := postPaymentResult(t, handler, "2322.00", child.Token, resultSignature("2322.00", child.Token, "test-pass2"))
	if code != http.StatusOK || body != "OK"+child.Token {
		t.Fatalf("callback status=%d body=%q want %d/%q", code, body, http.StatusOK, "OK"+child.Token)
	}

	updatedChild, found, err := st.GetPaymentByToken(ctx, child.Token)
	if err != nil || !found {
		t.Fatalf("updated child payment found=%v err=%v", found, err)
	}
	if updatedChild.Status != domain.PaymentStatusPaid {
		t.Fatalf("child payment status=%s want=%s", updatedChild.Status, domain.PaymentStatusPaid)
	}

	subs, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{UserID: userID, ConnectorID: connectorID, Limit: 20})
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("subscriptions len=%d want=2", len(subs))
	}

	var renewal domain.Subscription
	found = false
	for _, sub := range subs {
		if sub.PaymentID == updatedChild.ID {
			renewal = sub
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("renewal subscription for child payment not found")
	}
	if !renewal.StartsAt.Equal(parentEnd) {
		t.Fatalf("renewal starts_at=%s want=%s", renewal.StartsAt, parentEnd)
	}
	expectedEnd := parentEnd.Add(30 * 24 * time.Hour)
	if !renewal.EndsAt.Equal(expectedEnd) {
		t.Fatalf("renewal ends_at=%s want=%s", renewal.EndsAt, expectedEnd)
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{
		TargetUserID: userID,
		ConnectorID:  connectorID,
		Page:         1,
		PageSize:     100,
	})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionActivated); got != 1 {
		t.Fatalf("subscription_activated count=%d want=1", got)
	}
	if details := findAuditEventDetails(events, domain.AuditActionSubscriptionActivated); !strings.Contains(details, "payment_id="+strconv.FormatInt(updatedChild.ID, 10)) {
		t.Fatalf("subscription_activated details=%q want payment_id=%d", details, updatedChild.ID)
	}
}

func TestRecurringRuntimeTriggerRebill_ReusesExistingPendingPayment(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-recurring-existing-pending")
	parentInv := "rec-parent-existing"

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          parentInv,
		UserID:         seedTelegramUser(t, ctx, st, 990099),
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	})
	parentPayment, found, err := st.GetPaymentByToken(ctx, parentInv)
	if err != nil || !found {
		t.Fatalf("parent payment not found: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         parentPayment.UserID,
		ConnectorID:    parentPayment.ConnectorID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       time.Now().UTC().Add(-24 * time.Hour),
		EndsAt:         time.Now().UTC().Add(24 * time.Hour),
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	subscriptions, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{UserID: parentPayment.UserID, Limit: 10})
	if err != nil || len(subscriptions) != 1 {
		t.Fatalf("subscriptions len=%d err=%v", len(subscriptions), err)
	}
	sub := subscriptions[0]

	seedPayment(t, ctx, st, domain.Payment{
		Provider:          "robokassa",
		ProviderPaymentID: "rebill_parent:" + parentInv,
		Status:            domain.PaymentStatusPending,
		Token:             "rebill-existing-pending",
		UserID:            parentPayment.UserID,
		ConnectorID:       connectorID,
		SubscriptionID:    sub.ID,
		ParentPaymentID:   parentPayment.ID,
		AmountRUB:         2322,
		AutoPayEnabled:    true,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	})

	appCtx := testApplicationForRecurring(t, st)
	payload, err := appCtx.recurringRebillService().TriggerRebill(ctx, sub.ID, "test_runtime")
	if err != nil {
		t.Fatalf("triggerRebill err=%v", err)
	}
	if !payload.OK {
		t.Fatalf("payload.OK = false, want true")
	}
	if !payload.Existing {
		t.Fatalf("payload.Existing = false, want true")
	}
	if payload.InvoiceID != "rebill-existing-pending" {
		t.Fatalf("invoice_id=%q want=%q", payload.InvoiceID, "rebill-existing-pending")
	}

	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{UserID: parentPayment.UserID, Limit: 20})
	if err != nil {
		t.Fatalf("list payments: %v", err)
	}
	pendingCount := 0
	for _, paymentRow := range payments {
		if paymentRow.SubscriptionID == sub.ID && paymentRow.Status == domain.PaymentStatusPending && strings.HasPrefix(paymentRow.Token, "rebill-") {
			pendingCount++
		}
	}
	if pendingCount != 1 {
		t.Fatalf("pending rebill payments=%d want=1", pendingCount)
	}
}

func TestProcessRecurringRebills_RecordsStalePendingShortPeriodOnce(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Now().UTC()

	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-stale-short-rebill",
		Name:          "short stale",
		PriceRUB:      3,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 180,
		IsActive:      true,
		CreatedAt:     now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-stale-short-rebill")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}
	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "stale-parent",
		UserID:         seedTelegramUser(t, ctx, st, 990123),
		ConnectorID:    connector.ID,
		AmountRUB:      3,
		AutoPayEnabled: true,
		CreatedAt:      now.Add(-5 * time.Minute),
		UpdatedAt:      now.Add(-5 * time.Minute),
	})
	parentPayment, found, err := st.GetPaymentByToken(ctx, "stale-parent")
	if err != nil || !found {
		t.Fatalf("parent payment found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         parentPayment.UserID,
		ConnectorID:    connector.ID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(-3 * time.Minute),
		EndsAt:         now.Add(-20 * time.Second),
		CreatedAt:      now.Add(-3 * time.Minute),
		UpdatedAt:      now.Add(-3 * time.Minute),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment: %v", err)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, parentPayment.UserID, connector.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}
	seedPayment(t, ctx, st, domain.Payment{
		Provider:          "robokassa",
		ProviderPaymentID: "rebill_parent:" + parentPayment.Token,
		Status:            domain.PaymentStatusPending,
		Token:             "stale-child",
		UserID:            parentPayment.UserID,
		ConnectorID:       connector.ID,
		SubscriptionID:    sub.ID,
		ParentPaymentID:   parentPayment.ID,
		AmountRUB:         3,
		AutoPayEnabled:    true,
		CreatedAt:         now.Add(-70 * time.Second),
		UpdatedAt:         now.Add(-70 * time.Second),
	})

	appCtx := testApplicationForRecurring(t, st)
	processRecurringRebills(ctx, appCtx)
	processRecurringRebills(ctx, appCtx)

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{
		TargetUserID: parentPayment.UserID,
		ConnectorID:  connector.ID,
		Action:       domain.AuditActionRebillPendingStale,
		Page:         1,
		PageSize:     10,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("rebill_pending_stale events=%d want=1", len(events))
	}
}

func TestProcessRecurringRebills_DoesNotCreateNewPaymentWhenAttemptBudgetExhausted(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Now().UTC()

	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-attempt-budget-exhausted",
		Name:          "3h recurring",
		PriceRUB:      3,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 3 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-attempt-budget-exhausted")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}
	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "attempt-budget-parent",
		UserID:         seedTelegramUser(t, ctx, st, 990124),
		ConnectorID:    connector.ID,
		AmountRUB:      3,
		AutoPayEnabled: true,
		CreatedAt:      now.Add(-4 * time.Hour),
		UpdatedAt:      now.Add(-4 * time.Hour),
	})
	parentPayment, found, err := st.GetPaymentByToken(ctx, "attempt-budget-parent")
	if err != nil || !found {
		t.Fatalf("parent payment found=%v err=%v", found, err)
	}
	endsAt := now.Add(59 * time.Minute)
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         parentPayment.UserID,
		ConnectorID:    connector.ID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(-2 * time.Hour),
		EndsAt:         endsAt,
		CreatedAt:      now.Add(-2 * time.Hour),
		UpdatedAt:      now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment: %v", err)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, parentPayment.UserID, connector.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}
	seedPayment(t, ctx, st, domain.Payment{
		Provider:          "robokassa",
		ProviderPaymentID: "rebill_parent:" + parentPayment.Token,
		Status:            domain.PaymentStatusFailed,
		Token:             "attempt-budget-failed-child",
		UserID:            parentPayment.UserID,
		ConnectorID:       connector.ID,
		SubscriptionID:    sub.ID,
		ParentPaymentID:   parentPayment.ID,
		AmountRUB:         3,
		AutoPayEnabled:    true,
		CreatedAt:         now.Add(-5 * time.Minute),
		UpdatedAt:         now.Add(-5 * time.Minute),
	})

	appCtx := testApplicationForRecurring(t, st)
	processRecurringRebills(ctx, appCtx)

	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{UserID: parentPayment.UserID, Limit: 20})
	if err != nil {
		t.Fatalf("ListPayments: %v", err)
	}
	failedCount := 0
	pendingCount := 0
	for _, paymentRow := range payments {
		if paymentRow.SubscriptionID != sub.ID || paymentRow.ParentPaymentID != parentPayment.ID {
			continue
		}
		switch paymentRow.Status {
		case domain.PaymentStatusFailed:
			failedCount++
		case domain.PaymentStatusPending:
			pendingCount++
		}
	}
	if failedCount != 1 {
		t.Fatalf("failed rebill payments=%d want=1", failedCount)
	}
	if pendingCount != 0 {
		t.Fatalf("pending rebill payments=%d want=0", pendingCount)
	}
}

func TestProcessRecurringRebills_DoesNotCreateNewPaymentWhenRebillAlreadyPaid(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Now().UTC()

	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-rebill-already-paid",
		Name:          "3h recurring",
		PriceRUB:      3,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 3 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-rebill-already-paid")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}
	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "rebill-already-paid-parent",
		UserID:         seedTelegramUser(t, ctx, st, 990125),
		ConnectorID:    connector.ID,
		AmountRUB:      3,
		AutoPayEnabled: true,
		CreatedAt:      now.Add(-4 * time.Hour),
		UpdatedAt:      now.Add(-4 * time.Hour),
	})
	parentPayment, found, err := st.GetPaymentByToken(ctx, "rebill-already-paid-parent")
	if err != nil || !found {
		t.Fatalf("parent payment found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         parentPayment.UserID,
		ConnectorID:    connector.ID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(-2 * time.Hour),
		EndsAt:         now.Add(59 * time.Minute),
		CreatedAt:      now.Add(-2 * time.Hour),
		UpdatedAt:      now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment: %v", err)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, parentPayment.UserID, connector.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}
	seedPayment(t, ctx, st, domain.Payment{
		Provider:          "robokassa",
		ProviderPaymentID: "robokassa:rebill-already-paid-child",
		Status:            domain.PaymentStatusPaid,
		Token:             "rebill-already-paid-child",
		UserID:            parentPayment.UserID,
		ConnectorID:       connector.ID,
		SubscriptionID:    sub.ID,
		ParentPaymentID:   parentPayment.ID,
		AmountRUB:         3,
		AutoPayEnabled:    true,
		CreatedAt:         now.Add(-5 * time.Minute),
		UpdatedAt:         now.Add(-5 * time.Minute),
		PaidAt:            ptrTime(now.Add(-5 * time.Minute)),
	})

	appCtx := testApplicationForRecurring(t, st)
	processRecurringRebills(ctx, appCtx)

	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{UserID: parentPayment.UserID, Limit: 20})
	if err != nil {
		t.Fatalf("ListPayments: %v", err)
	}
	paidCount := 0
	pendingCount := 0
	for _, paymentRow := range payments {
		if paymentRow.SubscriptionID != sub.ID || paymentRow.ParentPaymentID != parentPayment.ID {
			continue
		}
		switch paymentRow.Status {
		case domain.PaymentStatusPaid:
			paidCount++
		case domain.PaymentStatusPending:
			pendingCount++
		}
	}
	if paidCount != 1 {
		t.Fatalf("paid rebill payments=%d want=1", paidCount)
	}
	if pendingCount != 0 {
		t.Fatalf("pending rebill payments=%d want=0", pendingCount)
	}
}

func testApplicationForRecurring(t *testing.T, st *memory.Store) *application {
	t.Helper()

	cfg := testRecurringConfig("")
	appCtx, err := newApplication(cfg, st, appInitOptions{ensureTelegramSetup: true})
	if err != nil {
		t.Fatalf("newApplication: %v", err)
	}
	return appCtx
}

func testRecurringConfig(rebillURL string) config.Config {
	cfg := config.Config{
		AppName:     "telega-bot-fedor-test",
		Environment: config.EnvLocal,
		HTTP: config.HTTPConfig{
			Address:      ":0",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
		Postgres: config.PostgresConfig{Driver: "memory"},
		Payment: config.PaymentConfig{
			Provider: "robokassa",
			Robokassa: config.RobokassaPaymentConfig{
				MerchantLogin:    "test-merchant",
				Password1:        "test-pass1",
				Password2:        "test-pass2",
				IsTestMode:       true,
				RecurringEnabled: true,
				RebillURL:        rebillURL,
			},
		},
	}
	return cfg
}
