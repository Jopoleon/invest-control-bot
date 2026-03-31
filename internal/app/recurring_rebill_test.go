package app

import (
	"context"
	"net/http"
	"net/http/httptest"
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
