package payments

import (
	"context"
	"errors"
	"strings"
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
		PaymentSuccessMessage:   func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
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

func TestActivateSuccessfulPayment_UsesTelegramAccessLinkWhenAvailable(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-payments-link",
		Name:          "private tg",
		PriceRUB:      1000,
		ChatID:        "1001234567890",
		ChannelURL:    "https://t.me/public_fallback",
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	paymentRow := domain.Payment{
		ID:          1,
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "p-access",
		UserID:      42,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}

	var sent messenger.OutgoingMessage
	var linkUserID int64
	var linkConnector domain.Connector
	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		BuildTelegramAccessLink: func(_ context.Context, userID int64, accessConnector domain.Connector) (string, error) {
			linkUserID = userID
			linkConnector = accessConnector
			return "https://t.me/+private_one_time_link", nil
		},
		ResolveConnectorChannel: func(a, b string) string { return a },
		SendUserNotification: func(_ context.Context, _ int64, _ string, msg messenger.OutgoingMessage) error {
			sent = msg
			return nil
		},
		BuildTargetAuditEvent: func(context.Context, int64, string, int64, string, string, time.Time) domain.AuditEvent {
			return domain.AuditEvent{}
		},
		SuccessChannelHint:     "",
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p-access", now)

	if len(sent.Buttons) == 0 || len(sent.Buttons[0]) == 0 {
		t.Fatalf("buttons are empty")
	}
	if linkUserID != paymentRow.UserID {
		t.Fatalf("link user id=%d want=%d", linkUserID, paymentRow.UserID)
	}
	if linkConnector.ID != connector.ID || linkConnector.ChatID != connector.ChatID {
		t.Fatalf("connector passed to access link builder = %+v want id=%d chat_id=%q", linkConnector, connector.ID, connector.ChatID)
	}
	if got := sent.Buttons[0][0].URL; got != "https://t.me/+private_one_time_link" {
		t.Fatalf("access link url=%q want telegram invite link", got)
	}
}

func TestActivateSuccessfulPayment_WritesAccessDeliveryFailedAuditWhenDestinationMissing(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 2, 10, 5, 0, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-payments-missing-access",
		Name:          "private tg",
		PriceRUB:      1000,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	paymentRow := domain.Payment{
		ID:          1,
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "p-no-access",
		UserID:      42,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}

	service := &Service{
		Store:                   st,
		PaymentSuccessMessage:   func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		ResolveConnectorChannel: func(a, b string) string { return "" },
		SendUserNotification:    func(context.Context, int64, string, messenger.OutgoingMessage) error { return nil },
		BuildTargetAuditEvent: func(_ context.Context, userID int64, _ string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p-no-access", now)

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListAuditEvents err=%v", err)
	}
	if got := countPaymentAuditEvents(events, domain.AuditActionAccessDeliveryFailed); got != 1 {
		t.Fatalf("access_delivery_failed count=%d want=1", got)
	}
	if !strings.Contains(findPaymentAuditDetails(events, domain.AuditActionAccessDeliveryFailed), "reason=missing_access_destination") {
		t.Fatalf("access_delivery_failed details=%q want missing_access_destination", findPaymentAuditDetails(events, domain.AuditActionAccessDeliveryFailed))
	}
}

func TestActivateSuccessfulPayment_WritesPaymentAccessReadyAuditWhenDeliverySucceeds(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 2, 10, 10, 0, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-payments-access-ready",
		Name:          "private tg",
		PriceRUB:      1000,
		ChannelURL:    "https://t.me/private_channel",
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	paymentRow := domain.Payment{
		ID:          1,
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "p-access-ready",
		UserID:      42,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}

	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		ResolveConnectorChannel: func(a, b string) string {
			return a
		},
		SendUserNotification: func(context.Context, int64, string, messenger.OutgoingMessage) error { return nil },
		BuildTargetAuditEvent: func(_ context.Context, userID int64, _ string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p-access-ready", now)

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListAuditEvents err=%v", err)
	}
	if got := countPaymentAuditEvents(events, domain.AuditActionPaymentAccessReady); got != 1 {
		t.Fatalf("payment_access_ready count=%d want=1", got)
	}
	if !strings.Contains(findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady), "source=connector_channel_url") {
		t.Fatalf("payment_access_ready details=%q want connector_channel_url", findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady))
	}
}

func TestActivateSuccessfulPayment_WritesInviteLinkFailureAuditWhenFallbackSucceeds(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 4, 2, 10, 12, 0, 0, time.UTC)
	connector := domain.Connector{
		ID:            1,
		StartPayload:  "in-payments-invite-fallback",
		Name:          "private tg",
		PriceRUB:      1000,
		ChatID:        "1001234567890",
		ChannelURL:    "https://t.me/private_channel",
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector err=%v", err)
	}
	paymentRow := domain.Payment{
		ID:          1,
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "p-invite-fallback",
		UserID:      42,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.CreatePayment(ctx, paymentRow); err != nil {
		t.Fatalf("CreatePayment err=%v", err)
	}

	service := &Service{
		Store:                 st,
		PaymentSuccessMessage: func(domain.Payment, domain.Connector, time.Time) string { return "ok" },
		BuildTelegramAccessLink: func(context.Context, int64, domain.Connector) (string, error) {
			return "", errors.New("telegram api failed")
		},
		ResolveConnectorChannel: func(a, b string) string {
			return a
		},
		SendUserNotification: func(context.Context, int64, string, messenger.OutgoingMessage) error { return nil },
		BuildTargetAuditEvent: func(_ context.Context, userID int64, _ string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		OpenChannelActionLabel: "open",
		MySubscriptionAction:   "sub",
	}

	service.ActivateSuccessfulPayment(ctx, paymentRow, "robokassa:p-invite-fallback", now)

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("ListAuditEvents err=%v", err)
	}
	if got := countPaymentAuditEvents(events, domain.AuditActionInviteLinkDeliveryFailed); got != 1 {
		t.Fatalf("invite_link_delivery_failed count=%d want=1", got)
	}
	if !strings.Contains(findPaymentAuditDetails(events, domain.AuditActionInviteLinkDeliveryFailed), "reason=invite_link_build_failed") {
		t.Fatalf("invite_link_delivery_failed details=%q want invite_link_build_failed", findPaymentAuditDetails(events, domain.AuditActionInviteLinkDeliveryFailed))
	}
	if got := countPaymentAuditEvents(events, domain.AuditActionPaymentAccessReady); got != 1 {
		t.Fatalf("payment_access_ready count=%d want=1", got)
	}
	if !strings.Contains(findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady), "source=connector_channel_url") {
		t.Fatalf("payment_access_ready details=%q want connector_channel_url", findPaymentAuditDetails(events, domain.AuditActionPaymentAccessReady))
	}
}

func countPaymentAuditEvents(events []domain.AuditEvent, action string) int {
	count := 0
	for _, event := range events {
		if event.Action == action {
			count++
		}
	}
	return count
}

func findPaymentAuditDetails(events []domain.AuditEvent, action string) string {
	for _, event := range events {
		if event.Action == action {
			return event.Details
		}
	}
	return ""
}
