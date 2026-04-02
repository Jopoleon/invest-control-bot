package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
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
	appCtx := &application{
		config:         config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:          st,
		telegramClient: tg,
	}

	processSubscriptionReminders(ctx, appCtx)
	processSubscriptionReminders(ctx, appCtx)

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
	appCtx := &application{
		config:         config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:          st,
		telegramClient: tg,
	}

	processExpiredSubscriptions(ctx, appCtx)
	processExpiredSubscriptions(ctx, appCtx)

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

func TestProcessExpiredSubscriptions_TelegramWritesRevokeAudit(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}

	connectorID := seedConnector(t, ctx, st, "in-lifecycle-expire-telegram-revoke")
	subscriptionID := seedActiveSubscription(t, ctx, st, connectorID, 880202, "sub-expire-telegram-revoke", time.Now().UTC().Add(-1*time.Minute))
	appCtx := &application{
		config:         config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:          st,
		telegramClient: tg,
	}

	processExpiredSubscriptions(ctx, appCtx)

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
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokedFromChat); got != 1 {
		t.Fatalf("subscription_revoked_from_chat count = %d, want 1", got)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokeFailed); got != 0 {
		t.Fatalf("subscription_revoke_failed count = %d, want 0", got)
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
	appCtx := &application{
		config:         config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:          st,
		telegramClient: tg,
	}

	processSubscriptionExpiryNotices(ctx, appCtx)
	processSubscriptionExpiryNotices(ctx, appCtx)

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

func TestProcessSubscriptionLifecycle_SkipsPreExpiryMessagesForShortTestPeriods(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}

	connectorID := seedShortPeriodConnector(t, ctx, st, "in-short-lifecycle-skip", 120)
	now := time.Now().UTC()
	subscriptionID := seedActiveSubscription(t, ctx, st, connectorID, 880004, "sub-short-lifecycle", now.Add(20*time.Second))
	appCtx := &application{
		config:         config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:          st,
		telegramClient: tg,
	}

	processSubscriptionReminders(ctx, appCtx)
	processSubscriptionExpiryNotices(ctx, appCtx)

	sub, found, err := st.GetSubscriptionByID(ctx, subscriptionID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	if !found {
		t.Fatalf("subscription not found")
	}
	if sub.ReminderSentAt != nil {
		t.Fatalf("reminder_sent_at = %v, want nil", sub.ReminderSentAt)
	}
	if sub.ExpiryNoticeSentAt != nil {
		t.Fatalf("expiry_notice_sent_at = %v, want nil", sub.ExpiryNoticeSentAt)
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionReminderSent); got != 0 {
		t.Fatalf("subscription_reminder_sent count = %d, want 0", got)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionExpiryNoticeSent); got != 0 {
		t.Fatalf("subscription_expiry_notice_sent count = %d, want 0", got)
	}
}

func TestProcessSubscriptionReminders_SendsMAXRenewalPrompt(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	maxSpy := &spySender{}

	connectorID := seedConnector(t, ctx, st, "in-max-lifecycle-reminder")
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "Федор Николаевич")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}
	seedActiveSubscriptionForUser(t, ctx, st, connectorID, maxUser.ID, 193465776, "sub-max-reminder", time.Now().UTC().Add(48*time.Hour))

	appCtx := &application{
		config:         config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:          st,
		telegramClient: tg,
		maxSender:      maxSpy,
	}

	processSubscriptionReminders(ctx, appCtx)

	if len(maxSpy.sent) != 1 {
		t.Fatalf("max sent messages = %d, want 1", len(maxSpy.sent))
	}
	if maxSpy.sent[0].user.Kind != messenger.KindMAX {
		t.Fatalf("sent kind = %s, want %s", maxSpy.sent[0].user.Kind, messenger.KindMAX)
	}
	if got := maxSpy.sent[0].msg.Text; !strings.Contains(got, "/start in-max-lifecycle-reminder") {
		t.Fatalf("reminder text = %q, want MAX renewal command", got)
	}
	if got := len(maxSpy.sent[0].msg.Buttons); got != 0 {
		t.Fatalf("button rows = %d, want 0 for MAX reminder prompt", got)
	}
}

func TestProcessSubscriptionReminders_SkipsWhenConnectorMissing(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}

	now := time.Now().UTC()
	seedPayment(t, ctx, st, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPaid,
		Token:       "sub-missing-connector",
		UserID:      seedTelegramUser(t, ctx, st, 880101),
		ConnectorID: 999999,
		AmountRUB:   2322,
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
		PaidAt:      &now,
	})

	paymentRow, found, err := st.GetPaymentByToken(ctx, "sub-missing-connector")
	if err != nil || !found {
		t.Fatalf("get payment by token: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:      paymentRow.UserID,
		ConnectorID: 999999,
		PaymentID:   paymentRow.ID,
		Status:      domain.SubscriptionStatusActive,
		StartsAt:    now.Add(-24 * time.Hour),
		EndsAt:      now.Add(48 * time.Hour),
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("upsert subscription: %v", err)
	}

	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, paymentRow.UserID, 999999)
	if err != nil || !found {
		t.Fatalf("get latest subscription: found=%v err=%v", found, err)
	}

	appCtx := &application{
		config:         config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:          st,
		telegramClient: tg,
	}

	processSubscriptionReminders(ctx, appCtx)

	sub, found, err = st.GetSubscriptionByID(ctx, sub.ID)
	if err != nil || !found {
		t.Fatalf("get subscription by id: found=%v err=%v", found, err)
	}
	if sub.ReminderSentAt != nil {
		t.Fatalf("reminder_sent_at = %v, want nil", sub.ReminderSentAt)
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionReminderSent); got != 0 {
		t.Fatalf("subscription_reminder_sent count = %d, want 0", got)
	}
}

func TestProcessExpiredSubscriptions_MAXDoesNotWriteTelegramRevokeAudit(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	maxSpy := &spySender{}

	connectorID := seedConnector(t, ctx, st, "in-expired-max-no-revoke")
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "Федор Николаевич")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}
	seedActiveSubscriptionForUser(t, ctx, st, connectorID, maxUser.ID, 193465776, "sub-max-expired", time.Now().UTC().Add(-1*time.Minute))

	appCtx := &application{
		config:         config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:          st,
		telegramClient: tg,
		maxSender:      maxSpy,
	}

	processExpiredSubscriptions(ctx, appCtx)

	if len(maxSpy.sent) != 1 {
		t.Fatalf("max sent messages = %d, want 1", len(maxSpy.sent))
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionExpired); got != 1 {
		t.Fatalf("subscription_expired count = %d, want 1", got)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokedFromChat); got != 0 {
		t.Fatalf("subscription_revoked_from_chat count = %d, want 0 for MAX", got)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokeFailed); got != 0 {
		t.Fatalf("subscription_revoke_failed count = %d, want 0 for MAX", got)
	}
}

func TestProcessExpiredSubscriptions_SkipsExpiredNotificationWhenRenewalAlreadyActive(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	maxSpy := &spySender{}
	now := time.Now().UTC()

	connectorID := seedConnector(t, ctx, st, "in-renewed-short-period")
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465777", "Федор Николаевич")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "sub-old-period",
		UserID:         maxUser.ID,
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      now.Add(-5 * time.Minute),
		UpdatedAt:      now.Add(-5 * time.Minute),
		PaidAt:         ptrTime(now.Add(-5 * time.Minute)),
	})
	oldPayment, found, err := st.GetPaymentByToken(ctx, "sub-old-period")
	if err != nil || !found {
		t.Fatalf("old payment found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         maxUser.ID,
		ConnectorID:    connectorID,
		PaymentID:      oldPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(-4 * time.Minute),
		EndsAt:         now.Add(-30 * time.Second),
		CreatedAt:      now.Add(-4 * time.Minute),
		UpdatedAt:      now.Add(-4 * time.Minute),
	}); err != nil {
		t.Fatalf("upsert old subscription: %v", err)
	}

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "sub-new-period",
		UserID:         maxUser.ID,
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      now.Add(-20 * time.Second),
		UpdatedAt:      now.Add(-20 * time.Second),
		PaidAt:         ptrTime(now.Add(-20 * time.Second)),
	})
	newPayment, found, err := st.GetPaymentByToken(ctx, "sub-new-period")
	if err != nil || !found {
		t.Fatalf("new payment found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         maxUser.ID,
		ConnectorID:    connectorID,
		PaymentID:      newPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(-20 * time.Second),
		EndsAt:         now.Add(3 * time.Minute),
		CreatedAt:      now.Add(-20 * time.Second),
		UpdatedAt:      now.Add(-20 * time.Second),
	}); err != nil {
		t.Fatalf("upsert new subscription: %v", err)
	}

	appCtx := &application{
		config:         config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:          st,
		telegramClient: tg,
		maxSender:      maxSpy,
	}

	processExpiredSubscriptions(ctx, appCtx)

	if len(maxSpy.sent) != 0 {
		t.Fatalf("expired notifications=%d want 0 when replacement active", len(maxSpy.sent))
	}
	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionExpired); got != 1 {
		t.Fatalf("subscription_expired count=%d want 1", got)
	}
}

func TestProcessExpiredSubscriptions_DefersShortPeriodExpiryWhilePendingRebillExists(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	maxSpy := &spySender{}
	now := time.Now().UTC()

	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465778", "Федор Николаевич")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}
	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-short-expiry-grace",
		Name:          "short recurring",
		PriceRUB:      2,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 4 * 60,
		IsActive:      true,
		CreatedAt:     now.Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-short-expiry-grace")
	if err != nil || !found {
		t.Fatalf("get connector found=%v err=%v", found, err)
	}

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "parent-short-grace",
		UserID:         maxUser.ID,
		ConnectorID:    connector.ID,
		AmountRUB:      2,
		AutoPayEnabled: true,
		CreatedAt:      now.Add(-5 * time.Minute),
		UpdatedAt:      now.Add(-5 * time.Minute),
		PaidAt:         ptrTime(now.Add(-5 * time.Minute)),
	})
	parentPayment, found, err := st.GetPaymentByToken(ctx, "parent-short-grace")
	if err != nil || !found {
		t.Fatalf("parent payment found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         maxUser.ID,
		ConnectorID:    connector.ID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(-4 * time.Minute),
		EndsAt:         now.Add(-10 * time.Second),
		CreatedAt:      now.Add(-4 * time.Minute),
		UpdatedAt:      now.Add(-4 * time.Minute),
	}); err != nil {
		t.Fatalf("upsert subscription: %v", err)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, maxUser.ID, connector.ID)
	if err != nil || !found {
		t.Fatalf("get latest subscription found=%v err=%v", found, err)
	}

	seedPayment(t, ctx, st, domain.Payment{
		Provider:          "robokassa",
		ProviderPaymentID: "rebill_parent:" + parentPayment.Token,
		Status:            domain.PaymentStatusPending,
		Token:             "pending-short-grace",
		UserID:            maxUser.ID,
		ConnectorID:       connector.ID,
		SubscriptionID:    sub.ID,
		ParentPaymentID:   parentPayment.ID,
		AmountRUB:         2,
		AutoPayEnabled:    true,
		CreatedAt:         now.Add(-20 * time.Second),
		UpdatedAt:         now.Add(-20 * time.Second),
	})

	appCtx := &application{
		config:         config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:          st,
		telegramClient: tg,
		maxSender:      maxSpy,
	}

	processExpiredSubscriptions(ctx, appCtx)

	sub, found, err = st.GetSubscriptionByID(ctx, sub.ID)
	if err != nil || !found {
		t.Fatalf("get subscription by id found=%v err=%v", found, err)
	}
	if sub.Status != domain.SubscriptionStatusActive {
		t.Fatalf("subscription status=%s want active during short-period grace", sub.Status)
	}
	if len(maxSpy.sent) != 0 {
		t.Fatalf("expired notifications=%d want 0 during grace", len(maxSpy.sent))
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func TestBuildRenewalNotification_WithoutPayloadLeavesPlainText(t *testing.T) {
	t.Helper()

	appCtx := &application{
		config: config.Config{
			Telegram: config.TelegramConfig{BotUsername: "test_bot"},
		},
	}

	msg := appCtx.buildRenewalNotification(context.Background(), 0, "880555", "", "test reminder")
	if msg.Text != "test reminder" {
		t.Fatalf("text = %q, want plain reminder text", msg.Text)
	}
	if len(msg.Buttons) != 0 {
		t.Fatalf("button rows = %d, want 0", len(msg.Buttons))
	}
}

func seedActiveSubscription(t *testing.T, ctx context.Context, st store.Store, connectorID, telegramID int64, paymentToken string, endsAt time.Time) int64 {
	t.Helper()
	return seedActiveSubscriptionForUser(t, ctx, st, connectorID, 0, telegramID, paymentToken, endsAt)
}

func seedActiveSubscriptionForUser(t *testing.T, ctx context.Context, st store.Store, connectorID, userID, telegramID int64, paymentToken string, endsAt time.Time) int64 {
	t.Helper()
	if userID <= 0 && telegramID > 0 {
		userID = seedTelegramUser(t, ctx, st, telegramID)
	}

	now := time.Now().UTC()
	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          paymentToken,
		UserID:         userID,
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
		UserID:         userID,
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

	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, paymentRow.UserID, connectorID)
	if err != nil {
		t.Fatalf("get latest subscription: %v", err)
	}
	if !found {
		t.Fatalf("subscription not found after upsert")
	}
	return sub.ID
}
