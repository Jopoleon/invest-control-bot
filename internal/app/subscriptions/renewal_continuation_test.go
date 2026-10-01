package subscriptions

import (
	"context"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

// renewalFixture reproduces the prod case where a user paid manually after the
// 3-day reminder: the current subscription keeps its ends_at and the renewal is
// queued as a second active row starting exactly at that ends_at.
type renewalFixture struct {
	st      *memory.Store
	userID  int64
	current domain.Subscription
	sent    []messenger.OutgoingMessage
}

func newRenewalFixture(t *testing.T, currentEndsIn time.Duration) *renewalFixture {
	t.Helper()
	ctx := context.Background()
	st := memory.New()
	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "92001", "renewed")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	f := &renewalFixture{st: st, userID: user.ID}
	connector := f.createConnector(t, "in-renewed-main", "-1001000000001")
	now := time.Now().UTC()
	f.current = f.createSubscription(t, connector.ID, "pay-current", now.Add(-30*24*time.Hour+currentEndsIn), now.Add(currentEndsIn))
	return f
}

func (f *renewalFixture) createConnector(t *testing.T, payload, chatID string) domain.Connector {
	t.Helper()
	ctx := context.Background()
	if err := f.st.CreateConnector(ctx, domain.Connector{
		Name:         payload,
		StartPayload: payload,
		PriceRUB:     5000,
		PeriodMode:   domain.ConnectorPeriodModeCalendarMonths,
		PeriodMonths: 1,
		ChatID:       chatID,
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := f.st.GetConnectorByStartPayload(ctx, payload)
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}
	return connector
}

func (f *renewalFixture) createSubscription(t *testing.T, connectorID int64, token string, startsAt, endsAt time.Time) domain.Subscription {
	t.Helper()
	ctx := context.Background()
	paidAt := time.Now().UTC()
	if err := f.st.CreatePayment(ctx, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPaid,
		Token:       token,
		UserID:      f.userID,
		ConnectorID: connectorID,
		AmountRUB:   5000,
		CreatedAt:   paidAt,
		UpdatedAt:   paidAt,
		PaidAt:      &paidAt,
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	payment, found, err := f.st.GetPaymentByToken(ctx, token)
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	if err := f.st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:      f.userID,
		ConnectorID: connectorID,
		PaymentID:   payment.ID,
		Status:      domain.SubscriptionStatusActive,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
		CreatedAt:   startsAt,
		UpdatedAt:   paidAt,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment: %v", err)
	}
	subs, err := f.st.ListSubscriptions(ctx, domain.SubscriptionListQuery{UserID: f.userID, Limit: 100})
	if err != nil {
		t.Fatalf("ListSubscriptions: %v", err)
	}
	for _, sub := range subs {
		if sub.PaymentID == payment.ID {
			return sub
		}
	}
	t.Fatalf("subscription for payment %d not found", payment.ID)
	return domain.Subscription{}
}

func (f *renewalFixture) service() *Service {
	return &Service{
		Store:                 f.st,
		TelegramBotUsername:   "test_bot",
		ReminderDaysBeforeEnd: 3,
		ExpiryNoticeWindow:    24 * time.Hour,
		SubscriptionJobLimit:  10,
		RenewalButtonLabel:    "Продлить подписку",
		RenewalCommandFormat:  "\n\n/start %s",
		SubscriptionReminderMessage: func(time.Time) string {
			return "reminder"
		},
		SubscriptionExpiryMessage: func(time.Time) string {
			return "expiry"
		},
		SendUserNotification: func(_ context.Context, _ int64, _ string, msg messenger.OutgoingMessage) error {
			f.sent = append(f.sent, msg)
			return nil
		},
		BuildTargetAuditEvent: func(_ context.Context, userID int64, preferred string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, TargetMessengerUserID: preferred, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		ResolvePreferredKind: func(context.Context, int64, string) messenger.Kind { return messenger.KindTelegram },
	}
}

func (f *renewalFixture) reloadCurrent(t *testing.T) domain.Subscription {
	t.Helper()
	sub, found, err := f.st.GetSubscriptionByID(context.Background(), f.current.ID)
	if err != nil || !found {
		t.Fatalf("GetSubscriptionByID found=%v err=%v", found, err)
	}
	return sub
}

func (f *renewalFixture) auditCount(t *testing.T, action string) int {
	t.Helper()
	events, _, err := f.st.ListAuditEvents(context.Background(), domain.AuditEventListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	return countAction(events, action)
}

func TestProcessSubscriptionReminders_SkipsWhenRenewalQueued(t *testing.T) {
	f := newRenewalFixture(t, 48*time.Hour)
	f.createSubscription(t, f.current.ConnectorID, "pay-renewal", f.current.EndsAt, f.current.EndsAt.AddDate(0, 1, 0))

	f.service().ProcessSubscriptionReminders(context.Background())

	if len(f.sent) != 0 {
		t.Fatalf("sent=%d want 0 because renewal is already paid", len(f.sent))
	}
	if f.reloadCurrent(t).ReminderSentAt == nil {
		t.Fatalf("ReminderSentAt=nil want marked so the job does not reselect the row")
	}
	if got := f.auditCount(t, domain.AuditActionSubscriptionReminderSkipped); got != 1 {
		t.Fatalf("reminder skipped audit count=%d want 1", got)
	}
	if got := f.auditCount(t, domain.AuditActionSubscriptionReminderSent); got != 0 {
		t.Fatalf("reminder sent audit count=%d want 0", got)
	}
}

func TestProcessSubscriptionExpiryNotices_SkipsWhenRenewalQueued(t *testing.T) {
	f := newRenewalFixture(t, 12*time.Hour)
	f.createSubscription(t, f.current.ConnectorID, "pay-renewal", f.current.EndsAt, f.current.EndsAt.AddDate(0, 1, 0))

	f.service().ProcessSubscriptionExpiryNotices(context.Background())

	if len(f.sent) != 0 {
		t.Fatalf("sent=%d want 0 because renewal is already paid", len(f.sent))
	}
	if f.reloadCurrent(t).ExpiryNoticeSentAt == nil {
		t.Fatalf("ExpiryNoticeSentAt=nil want marked so the job does not reselect the row")
	}
	if got := f.auditCount(t, domain.AuditActionSubscriptionExpiryNoticeSkipped); got != 1 {
		t.Fatalf("expiry notice skipped audit count=%d want 1", got)
	}
	if got := f.auditCount(t, domain.AuditActionSubscriptionExpiryNoticeSent); got != 0 {
		t.Fatalf("expiry notice sent audit count=%d want 0", got)
	}
}

func TestProcessSubscriptionExpiryNotices_SkipsWhenSameChatRenewedViaOtherConnector(t *testing.T) {
	f := newRenewalFixture(t, 12*time.Hour)
	other := f.createConnector(t, "in-renewed-other", "-1001000000001")
	f.createSubscription(t, other.ID, "pay-renewal-other", f.current.EndsAt.Add(-time.Hour), f.current.EndsAt.AddDate(0, 1, 0))

	f.service().ProcessSubscriptionExpiryNotices(context.Background())

	if len(f.sent) != 0 {
		t.Fatalf("sent=%d want 0 because the same chat is already renewed", len(f.sent))
	}
	if got := f.auditCount(t, domain.AuditActionSubscriptionExpiryNoticeSkipped); got != 1 {
		t.Fatalf("expiry notice skipped audit count=%d want 1", got)
	}
}

func TestProcessSubscriptionExpiryNotices_SendsWhenOtherSubscriptionDoesNotContinueAccess(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, f *renewalFixture)
	}{
		{
			name: "different chat",
			setup: func(t *testing.T, f *renewalFixture) {
				other := f.createConnector(t, "in-other-chat", "-1001000000002")
				f.createSubscription(t, other.ID, "pay-other-chat", f.current.EndsAt, f.current.EndsAt.AddDate(0, 1, 0))
			},
		},
		{
			name: "renewal starts after a gap",
			setup: func(t *testing.T, f *renewalFixture) {
				f.createSubscription(t, f.current.ConnectorID, "pay-gap", f.current.EndsAt.Add(time.Hour), f.current.EndsAt.AddDate(0, 1, 0))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRenewalFixture(t, 12*time.Hour)
			tt.setup(t, f)

			f.service().ProcessSubscriptionExpiryNotices(context.Background())

			if len(f.sent) != 1 {
				t.Fatalf("sent=%d want 1", len(f.sent))
			}
			if got := f.auditCount(t, domain.AuditActionSubscriptionExpiryNoticeSent); got != 1 {
				t.Fatalf("expiry notice sent audit count=%d want 1", got)
			}
		})
	}
}
