package admin

import (
	"context"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestSummarizeAutopayFromSubscriptions_PrefersCurrentPeriodOverFutureRenewal(t *testing.T) {
	now := time.Now().UTC()

	enabled, configured := summarizeAutopayFromSubscriptions([]domain.Subscription{
		{
			ID:             1,
			Status:         domain.SubscriptionStatusActive,
			AutoPayEnabled: false,
			StartsAt:       now.Add(-time.Hour),
			EndsAt:         now.Add(time.Hour),
		},
		{
			ID:             2,
			Status:         domain.SubscriptionStatusActive,
			AutoPayEnabled: true,
			StartsAt:       now.Add(time.Hour),
			EndsAt:         now.Add(2 * time.Hour),
		},
	}, now)

	if enabled {
		t.Fatalf("enabled = true, want false because current period wins over future renewal")
	}
	if !configured {
		t.Fatalf("configured = false, want true")
	}
}

func TestSummarizeAutopayFromSubscriptions_FallsBackToFutureRenewalWhenNoCurrentPeriod(t *testing.T) {
	now := time.Now().UTC()

	enabled, configured := summarizeAutopayFromSubscriptions([]domain.Subscription{
		{
			ID:             2,
			Status:         domain.SubscriptionStatusActive,
			AutoPayEnabled: true,
			StartsAt:       now.Add(time.Hour),
			EndsAt:         now.Add(2 * time.Hour),
		},
	}, now)

	if !enabled {
		t.Fatalf("enabled = false, want true from future renewal fallback")
	}
	if !configured {
		t.Fatalf("configured = false, want true")
	}
}

func TestSelectOperationalSubscription_PrefersCurrentThenNearestFutureThenHistory(t *testing.T) {
	now := time.Now().UTC()

	current, ok := selectOperationalSubscription([]domain.Subscription{
		{
			ID:        10,
			Status:    domain.SubscriptionStatusExpired,
			StartsAt:  now.Add(-4 * time.Hour),
			EndsAt:    now.Add(-2 * time.Hour),
			UpdatedAt: now.Add(-2 * time.Hour),
		},
		{
			ID:        11,
			Status:    domain.SubscriptionStatusActive,
			StartsAt:  now.Add(time.Hour),
			EndsAt:    now.Add(2 * time.Hour),
			UpdatedAt: now.Add(-time.Minute),
		},
		{
			ID:        12,
			Status:    domain.SubscriptionStatusActive,
			StartsAt:  now.Add(-time.Hour),
			EndsAt:    now.Add(time.Hour),
			UpdatedAt: now.Add(-3 * time.Minute),
		},
	}, now)
	if !ok {
		t.Fatal("selectOperationalSubscription() found=false, want true")
	}
	if current.ID != 12 {
		t.Fatalf("selected ID = %d, want current subscription 12", current.ID)
	}

	future, ok := selectOperationalSubscription([]domain.Subscription{
		{
			ID:        21,
			Status:    domain.SubscriptionStatusActive,
			StartsAt:  now.Add(3 * time.Hour),
			EndsAt:    now.Add(4 * time.Hour),
			UpdatedAt: now,
		},
		{
			ID:        22,
			Status:    domain.SubscriptionStatusActive,
			StartsAt:  now.Add(time.Hour),
			EndsAt:    now.Add(2 * time.Hour),
			UpdatedAt: now.Add(-time.Minute),
		},
	}, now)
	if !ok {
		t.Fatal("selectOperationalSubscription() future found=false, want true")
	}
	if future.ID != 22 {
		t.Fatalf("selected ID = %d, want nearest future subscription 22", future.ID)
	}
}

func TestSortSubscriptionsForOperationalView_OrdersCurrentThenFutureThenHistory(t *testing.T) {
	now := time.Now().UTC()
	subs := []domain.Subscription{
		{
			ID:        1,
			Status:    domain.SubscriptionStatusExpired,
			StartsAt:  now.Add(-6 * time.Hour),
			EndsAt:    now.Add(-5 * time.Hour),
			UpdatedAt: now.Add(-time.Minute),
		},
		{
			ID:        2,
			Status:    domain.SubscriptionStatusActive,
			StartsAt:  now.Add(2 * time.Hour),
			EndsAt:    now.Add(3 * time.Hour),
			UpdatedAt: now.Add(-2 * time.Minute),
		},
		{
			ID:        3,
			Status:    domain.SubscriptionStatusActive,
			StartsAt:  now.Add(-2 * time.Hour),
			EndsAt:    now.Add(2 * time.Hour),
			UpdatedAt: now.Add(-3 * time.Minute),
		},
		{
			ID:        4,
			Status:    domain.SubscriptionStatusActive,
			StartsAt:  now.Add(time.Hour),
			EndsAt:    now.Add(2 * time.Hour),
			UpdatedAt: now.Add(-4 * time.Minute),
		},
		{
			ID:        5,
			Status:    domain.SubscriptionStatusActive,
			StartsAt:  now.Add(-time.Hour),
			EndsAt:    now.Add(time.Hour),
			UpdatedAt: now.Add(-5 * time.Minute),
		},
	}

	sortSubscriptionsForOperationalView(subs, now)

	got := []int64{subs[0].ID, subs[1].ID, subs[2].ID, subs[3].ID, subs[4].ID}
	want := []int64{5, 3, 4, 2, 1}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("sorted IDs = %v, want %v", got, want)
		}
	}
}

func TestBuildChurnIssues_PrefersCurrentSubscriptionForAutopayState(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)
	now := time.Now().UTC()

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "123456789", "egor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-churn-current-vs-future",
		Name:          "Test Connector",
		ChannelURL:    "https://t.me/testconnector",
		PriceRUB:      100,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 3600,
		IsActive:      true,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-churn-current-vs-future")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}

	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusFailed,
		Token:       "failed-payment",
		UserID:      user.ID,
		ConnectorID: connector.ID,
		AmountRUB:   connector.PriceRUB,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("CreatePayment failed: %v", err)
	}
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPaid,
		Token:       "current-payment",
		UserID:      user.ID,
		ConnectorID: connector.ID,
		AmountRUB:   connector.PriceRUB,
		CreatedAt:   now.Add(-2 * time.Hour),
		UpdatedAt:   now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("CreatePayment current: %v", err)
	}
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPaid,
		Token:       "future-payment",
		UserID:      user.ID,
		ConnectorID: connector.ID,
		AmountRUB:   connector.PriceRUB,
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("CreatePayment future: %v", err)
	}
	currentPayment, found, err := st.GetPaymentByToken(ctx, "current-payment")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken current found=%v err=%v", found, err)
	}
	futurePayment, found, err := st.GetPaymentByToken(ctx, "future-payment")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken future found=%v err=%v", found, err)
	}

	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         user.ID,
		ConnectorID:    connector.ID,
		PaymentID:      currentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: false,
		StartsAt:       now.Add(-time.Hour),
		EndsAt:         now.Add(time.Hour),
		CreatedAt:      now.Add(-2 * time.Hour),
		UpdatedAt:      now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment current: %v", err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         user.ID,
		ConnectorID:    connector.ID,
		PaymentID:      futurePayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(time.Hour),
		EndsAt:         now.Add(2 * time.Hour),
		CreatedAt:      now.Add(-time.Hour),
		UpdatedAt:      now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment future: %v", err)
	}

	issues, err := h.buildChurnIssues(ctx, "ru", user.ID, "", "", "", string(churnIssueFailedPayment), "", "")
	if err != nil {
		t.Fatalf("buildChurnIssues: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("issue count = %d, want 1", len(issues))
	}

	disabledLabel, _ := autoPayBadge("ru", false, true)
	if issues[0].AutoPayLabel != disabledLabel {
		t.Fatalf("AutoPayLabel = %q, want %q from current period", issues[0].AutoPayLabel, disabledLabel)
	}
}
