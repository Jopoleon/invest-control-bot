package app

import (
	"context"
	"testing"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/Jopoleon/telega-bot-fedor/internal/store"
	"github.com/Jopoleon/telega-bot-fedor/internal/store/memory"
	"github.com/Jopoleon/telega-bot-fedor/internal/telegram"
)

func TestProcessSubscriptionReminders_MarksReminderOnce(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}

	connectorID := seedConnector(t, ctx, st, "in-lifecycle-reminder")
	subscriptionID := seedActiveSubscription(t, ctx, st, connectorID, 880001, "sub-reminder", time.Now().UTC().Add(48*time.Hour))

	processSubscriptionReminders(ctx, st, tg, "test_bot")
	processSubscriptionReminders(ctx, st, tg, "test_bot")

	sub, found, err := st.GetSubscriptionByID(ctx, subscriptionID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	if !found {
		t.Fatalf("subscription not found")
	}
	if sub.ReminderSentAt == nil {
		t.Fatalf("reminder_sent_at is nil")
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionReminderSent); got != 1 {
		t.Fatalf("subscription_reminder_sent count = %d, want 1", got)
	}
}

func TestProcessExpiredSubscriptions_ExpiresOnce(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}

	connectorID := seedConnector(t, ctx, st, "in-lifecycle-expire")
	subscriptionID := seedActiveSubscription(t, ctx, st, connectorID, 880002, "sub-expire", time.Now().UTC().Add(-1*time.Minute))

	processExpiredSubscriptions(ctx, st, tg, "test_bot")
	processExpiredSubscriptions(ctx, st, tg, "test_bot")

	sub, found, err := st.GetSubscriptionByID(ctx, subscriptionID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	if !found {
		t.Fatalf("subscription not found")
	}
	if sub.Status != domain.SubscriptionStatusExpired {
		t.Fatalf("subscription status = %s, want %s", sub.Status, domain.SubscriptionStatusExpired)
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionExpired); got != 1 {
		t.Fatalf("subscription_expired count = %d, want 1", got)
	}
}

func TestProcessSubscriptionExpiryNotices_MarksNoticeOnce(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}

	connectorID := seedConnector(t, ctx, st, "in-lifecycle-expiry-notice")
	subscriptionID := seedActiveSubscription(t, ctx, st, connectorID, 880003, "sub-expiry-notice", time.Now().UTC().Add(12*time.Hour))

	processSubscriptionExpiryNotices(ctx, st, tg, "test_bot")
	processSubscriptionExpiryNotices(ctx, st, tg, "test_bot")

	sub, found, err := st.GetSubscriptionByID(ctx, subscriptionID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	if !found {
		t.Fatalf("subscription not found")
	}
	if sub.ExpiryNoticeSentAt == nil {
		t.Fatalf("expiry_notice_sent_at is nil")
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionExpiryNoticeSent); got != 1 {
		t.Fatalf("subscription_expiry_notice_sent count = %d, want 1", got)
	}
}

func seedActiveSubscription(t *testing.T, ctx context.Context, st store.Store, connectorID, telegramID int64, paymentToken string, endsAt time.Time) int64 {
	t.Helper()

	now := time.Now().UTC()
	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          paymentToken,
		TelegramID:     telegramID,
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: false,
		CreatedAt:      now.Add(-24 * time.Hour),
		UpdatedAt:      now.Add(-24 * time.Hour),
		PaidAt:         &now,
	})

	paymentRow, found, err := st.GetPaymentByToken(ctx, paymentToken)
	if err != nil {
		t.Fatalf("get payment by token: %v", err)
	}
	if !found {
		t.Fatalf("payment not found by token=%s", paymentToken)
	}

	startsAt := endsAt.Add(-30 * 24 * time.Hour)
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		TelegramID:     telegramID,
		ConnectorID:    connectorID,
		PaymentID:      paymentRow.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: false,
		StartsAt:       startsAt,
		EndsAt:         endsAt,
		CreatedAt:      startsAt,
		UpdatedAt:      startsAt,
	}); err != nil {
		t.Fatalf("upsert subscription: %v", err)
	}

	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, telegramID, connectorID)
	if err != nil {
		t.Fatalf("get latest subscription: %v", err)
	}
	if !found {
		t.Fatalf("subscription not found after upsert")
	}
	return sub.ID
}
