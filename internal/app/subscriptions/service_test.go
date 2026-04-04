package subscriptions

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestBuildRenewalNotification_MAXUsesCommandText(t *testing.T) {
	svc := &Service{
		TelegramBotUsername:  "test_bot",
		RenewalButtonLabel:   "Продлить подписку",
		RenewalCommandFormat: "\n\nДля продления отправьте команду:\n/start %s",
		ResolvePreferredKind: func(context.Context, int64, string) messenger.Kind { return messenger.KindMAX },
	}

	msg := svc.BuildRenewalNotification(context.Background(), 1, "193465776", "in-max-renew", "Напоминание")
	if !strings.Contains(msg.Text, "/start in-max-renew") {
		t.Fatalf("text=%q want max command", msg.Text)
	}
	if len(msg.Buttons) != 0 {
		t.Fatalf("button rows=%d want=0", len(msg.Buttons))
	}
}

func TestBuildRenewalNotification_TelegramUsesButton(t *testing.T) {
	svc := &Service{
		TelegramBotUsername:  "test_bot",
		RenewalButtonLabel:   "Продлить подписку",
		RenewalCommandFormat: "\n\nДля продления отправьте команду:\n/start %s",
		ResolvePreferredKind: func(context.Context, int64, string) messenger.Kind { return messenger.KindTelegram },
	}

	msg := svc.BuildRenewalNotification(context.Background(), 1, "777", "in-tg-renew", "Напоминание")
	if len(msg.Buttons) != 1 || len(msg.Buttons[0]) != 1 {
		t.Fatalf("buttons=%v want one telegram renewal button", msg.Buttons)
	}
	if msg.Buttons[0][0].URL != "https://t.me/test_bot?start=in-tg-renew" {
		t.Fatalf("url=%q want telegram deeplink", msg.Buttons[0][0].URL)
	}
}

func TestShouldSendReminder_SkipsShortTestPeriods(t *testing.T) {
	connector := domain.Connector{PeriodMode: domain.ConnectorPeriodModeDuration, PeriodSeconds: 120}

	if shouldSendReminder(connector) {
		t.Fatalf("shouldSendReminder(short period)=true want false")
	}
}

func TestShouldSendExpiryNotice_SkipsShortTestPeriods(t *testing.T) {
	if shouldSendExpiryNotice(domain.Connector{PeriodMode: domain.ConnectorPeriodModeDuration, PeriodSeconds: 120}) {
		t.Fatalf("shouldSendExpiryNotice(short period)=true want false")
	}
	if !shouldSendExpiryNotice(domain.Connector{PeriodMode: domain.ConnectorPeriodModeDuration, PeriodSeconds: 30 * 24 * 60 * 60}) {
		t.Fatalf("shouldSendExpiryNotice(normal period)=false want true")
	}
}

func TestProcessSubscriptionReminders_SendsAndMarksAudit(t *testing.T) {
	ctx := context.Background()
	st := memory.New()

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "91001", "egor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	if err := st.CreateConnector(ctx, domain.Connector{
		Name:          "Reminder tariff",
		StartPayload:  "in-reminder",
		PriceRUB:      2322,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-reminder")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}

	now := time.Now().UTC()
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPaid,
		Token:       "pay-reminder-1",
		UserID:      user.ID,
		ConnectorID: connector.ID,
		AmountRUB:   2322,
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now.Add(-time.Hour),
		PaidAt:      &now,
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	paymentRow, found, err := st.GetPaymentByToken(ctx, "pay-reminder-1")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:      user.ID,
		ConnectorID: connector.ID,
		PaymentID:   paymentRow.ID,
		Status:      domain.SubscriptionStatusActive,
		StartsAt:    now.Add(-24 * time.Hour),
		EndsAt:      now.Add(48 * time.Hour),
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment: %v", err)
	}

	var sent []messenger.OutgoingMessage
	svc := &Service{
		Store:                 st,
		TelegramBotUsername:   "test_bot",
		ReminderDaysBeforeEnd: 3,
		SubscriptionJobLimit:  10,
		RenewalButtonLabel:    "Продлить подписку",
		RenewalCommandFormat:  "\n\nДля продления отправьте команду:\n/start %s",
		SubscriptionReminderMessage: func(time.Time) string {
			return "Напоминание"
		},
		SendUserNotification: func(_ context.Context, _ int64, _ string, msg messenger.OutgoingMessage) error {
			sent = append(sent, msg)
			return nil
		},
		BuildTargetAuditEvent: func(_ context.Context, userID int64, preferred string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, TargetMessengerUserID: preferred, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		ResolvePreferredKind: func(context.Context, int64, string) messenger.Kind { return messenger.KindTelegram },
	}

	svc.ProcessSubscriptionReminders(ctx)

	if len(sent) != 1 {
		t.Fatalf("sent=%d want 1", len(sent))
	}
	if len(sent[0].Buttons) != 1 || sent[0].Buttons[0][0].URL != "https://t.me/test_bot?start=in-reminder" {
		t.Fatalf("buttons=%v want telegram renewal button", sent[0].Buttons)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, user.ID, connector.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}
	if sub.ReminderSentAt == nil {
		t.Fatalf("ReminderSentAt=nil want marked timestamp")
	}
	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	if got := countAction(events, domain.AuditActionSubscriptionReminderSent); got != 1 {
		t.Fatalf("subscription_reminder_sent count=%d want 1", got)
	}
}

func TestProcessExpiredSubscriptions_DefersWhenPendingRebillExists(t *testing.T) {
	ctx := context.Background()
	st := memory.New()

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "91002", "egor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	if err := st.CreateConnector(ctx, domain.Connector{
		Name:          "Short tariff",
		StartPayload:  "in-short-expire",
		PriceRUB:      99,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 180,
		ChatID:        "-1001234567890",
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-short-expire")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}

	now := time.Now().UTC()
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "pay-expire-parent",
		UserID:         user.ID,
		ConnectorID:    connector.ID,
		AmountRUB:      99,
		AutoPayEnabled: true,
		CreatedAt:      now.Add(-5 * time.Minute),
		UpdatedAt:      now.Add(-5 * time.Minute),
		PaidAt:         &now,
	}); err != nil {
		t.Fatalf("CreatePayment(parent): %v", err)
	}
	parent, found, err := st.GetPaymentByToken(ctx, "pay-expire-parent")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken parent found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         user.ID,
		ConnectorID:    connector.ID,
		PaymentID:      parent.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(-3 * time.Minute),
		EndsAt:         now.Add(-20 * time.Second),
		CreatedAt:      now.Add(-3 * time.Minute),
		UpdatedAt:      now.Add(-3 * time.Minute),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment: %v", err)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, user.ID, connector.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:        "robokassa",
		Status:          domain.PaymentStatusPending,
		Token:           "pending-rebill-1",
		UserID:          user.ID,
		ConnectorID:     connector.ID,
		SubscriptionID:  sub.ID,
		ParentPaymentID: parent.ID,
		AmountRUB:       99,
		AutoPayEnabled:  true,
		CreatedAt:       now.Add(-30 * time.Second),
		UpdatedAt:       now.Add(-30 * time.Second),
	}); err != nil {
		t.Fatalf("CreatePayment(pending): %v", err)
	}

	svc := &Service{
		Store:                   st,
		SubscriptionJobLimit:    10,
		SubscriptionExpiredText: "expired",
		SendUserNotification: func(context.Context, int64, string, messenger.OutgoingMessage) error {
			t.Fatalf("SendUserNotification should not be called when expiration is deferred")
			return nil
		},
		BuildTargetAuditEvent: func(_ context.Context, userID int64, preferred string, connectorID int64, action, details string, createdAt time.Time) domain.AuditEvent {
			return domain.AuditEvent{TargetUserID: userID, TargetMessengerUserID: preferred, ConnectorID: connectorID, Action: action, Details: details, CreatedAt: createdAt}
		},
		ResolvePreferredKind: func(context.Context, int64, string) messenger.Kind { return messenger.KindTelegram },
	}

	svc.ProcessExpiredSubscriptions(ctx)

	sub, found, err = st.GetSubscriptionByID(ctx, sub.ID)
	if err != nil || !found {
		t.Fatalf("GetSubscriptionByID found=%v err=%v", found, err)
	}
	if sub.Status != domain.SubscriptionStatusActive {
		t.Fatalf("status=%s want active because pending rebill should defer expiration", sub.Status)
	}
	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	if got := countAction(events, domain.AuditActionSubscriptionExpired); got != 0 {
		t.Fatalf("subscription_expired count=%d want 0", got)
	}
}

func countAction(events []domain.AuditEvent, action string) int {
	count := 0
	for _, event := range events {
		if event.Action == action {
			count++
		}
	}
	return count
}
