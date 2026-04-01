package payments

import (
	"context"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestNotifyFailedRecurringPayment_SkipsWhenAutopayDisabled(t *testing.T) {
	st := memory.New()
	sent := 0
	service := &Service{
		Store:                 st,
		TelegramBotUsername:   "test_bot",
		FailedRecurringText:   "fail",
		FailedRecurringButton: "pay",
		SendUserNotification:  func(context.Context, int64, string, messenger.OutgoingMessage) error { sent++; return nil },
		BuildTargetAuditEvent: func(context.Context, int64, string, int64, string, string, time.Time) domain.AuditEvent {
			return domain.AuditEvent{}
		},
	}

	service.NotifyFailedRecurringPayment(context.Background(), domain.Payment{ID: 1, UserID: 10, ConnectorID: 1, SubscriptionID: 2, AutoPayEnabled: false})
	if sent != 0 {
		t.Fatalf("sent=%d want=0", sent)
	}
}

func TestActivateSuccessfulPayment_ExtendsFromCurrentSubscriptionEnd(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	connector := domain.Connector{ID: 1, StartPayload: "in-payments-pkg", Name: "tariff", PriceRUB: 1000, PeriodMode: domain.ConnectorPeriodModeDuration, PeriodSeconds: 30 * 24 * 60 * 60, IsActive: true, CreatedAt: now}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	paymentRow := domain.Payment{ID: 1, Provider: "robokassa", Status: domain.PaymentStatusPending, Token: "p1", UserID: 42, ConnectorID: connector.ID, AmountRUB: 1000, CreatedAt: now, UpdatedAt: now}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{UserID: 42, ConnectorID: connector.ID, PaymentID: 999, Status: domain.SubscriptionStatusActive, StartsAt: now.Add(-24 * time.Hour), EndsAt: now.Add(10 * 24 * time.Hour), CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment err=%v", err)
	}

	service := &Service{
		Store:                   st,
		PaymentSuccessMessage:   func(time.Time) string { return "ok" },
		ResolveConnectorChannel: func(a, b string) string { return "" },
		SendUserNotification:    func(context.Context, int64, string, messenger.OutgoingMessage) error { return nil },
		BuildTargetAuditEvent: func(context.Context, int64, string, int64, string, string, time.Time) domain.AuditEvent {
			return domain.AuditEvent{}
		},
		SuccessChannelHint:     "",
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p1", now)

	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, 42, connector.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}
	wantStart := now.Add(10 * 24 * time.Hour)
	if !sub.StartsAt.Equal(wantStart) {
		t.Fatalf("starts_at=%s want=%s", sub.StartsAt, wantStart)
	}
}
