package app

import (
	"context"
	"errors"
	"strconv"
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

func TestProcessSubscriptionReminders_SkipsFutureQueuedSubscription(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}

	now := time.Now().UTC()
	connectorID := seedConnector(t, ctx, st, "in-lifecycle-reminder-future")
	userID := seedTelegramUser(t, ctx, st, 880011)
	seedPayment(t, ctx, st, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPaid,
		Token:       "sub-reminder-future",
		UserID:      userID,
		ConnectorID: connectorID,
		AmountRUB:   2322,
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now.Add(-time.Hour),
		PaidAt:      &now,
	})
	paymentRow, found, err := st.GetPaymentByToken(ctx, "sub-reminder-future")
	if err != nil || !found {
		t.Fatalf("get payment by token: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:      userID,
		ConnectorID: connectorID,
		PaymentID:   paymentRow.ID,
		Status:      domain.SubscriptionStatusActive,
		StartsAt:    now.Add(12 * time.Hour),
		EndsAt:      now.Add(48 * time.Hour),
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("upsert subscription: %v", err)
	}

	appCtx := &application{
		config:         config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:          st,
		telegramClient: tg,
	}

	processSubscriptionReminders(ctx, appCtx)

	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, userID, connectorID)
	if err != nil || !found {
		t.Fatalf("get latest subscription: found=%v err=%v", found, err)
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

func TestProcessExpiredSubscriptions_SkipsRevokeWhenOtherConnectorKeepsSameTelegramChatAccess(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()

	oldConnectorID := seedConnector(t, ctx, st, "expire-same-chat-old")
	newConnectorID := seedConnector(t, ctx, st, "expire-same-chat-new")
	userID := seedTelegramUser(t, ctx, st, 880204)
	oldSubscriptionID := seedActiveSubscriptionForUser(t, ctx, st, oldConnectorID, userID, 880204, "sub-expire-same-chat-old", time.Now().UTC().Add(-1*time.Minute))
	seedActiveSubscriptionForUser(t, ctx, st, newConnectorID, userID, 880204, "sub-expire-same-chat-new", time.Now().UTC().Add(29*24*time.Hour))

	appCtx := &application{
		config: config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:  st,
	}
	svc := appCtx.subscriptionLifecycleService()
	revokeCalls := 0
	svc.RemoveTelegramChatMember = func(context.Context, int64, int64) error {
		revokeCalls++
		return nil
	}
	notifications := 0
	svc.SendUserNotification = func(context.Context, int64, string, messenger.OutgoingMessage) error {
		notifications++
		return nil
	}

	svc.ProcessExpiredSubscriptions(ctx)

	sub, found, err := st.GetSubscriptionByID(ctx, oldSubscriptionID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	if !found {
		t.Fatalf("subscription not found")
	}
	if sub.Status != domain.SubscriptionStatusExpired {
		t.Fatalf("subscription status = %s, want %s", sub.Status, domain.SubscriptionStatusExpired)
	}
	if revokeCalls != 0 {
		t.Fatalf("revoke calls = %d, want 0", revokeCalls)
	}
	if notifications != 0 {
		t.Fatalf("notifications = %d, want 0", notifications)
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokedFromChat); got != 0 {
		t.Fatalf("subscription_revoked_from_chat count = %d, want 0", got)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionExpired); got != 1 {
		t.Fatalf("subscription_expired count = %d, want 1", got)
	}
	if details := findAuditEventDetails(events, domain.AuditActionSubscriptionExpired); !strings.Contains(details, "same_telegram_chat_active=true") {
		t.Fatalf("subscription_expired details=%q want same_telegram_chat_active=true", details)
	}
}

func TestProcessExpiredSubscriptions_WritesRetryableRevokeFailureAudit(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}

	connectorID := seedConnector(t, ctx, st, "in-lifecycle-expire-telegram-revoke-fail")
	subscriptionID := seedActiveSubscription(t, ctx, st, connectorID, 880203, "sub-expire-telegram-revoke-fail", time.Now().UTC().Add(-1*time.Minute))
	appCtx := &application{
		config:         config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:          st,
		telegramClient: tg,
	}
	svc := appCtx.subscriptionLifecycleService()
	svc.RemoveTelegramChatMember = func(context.Context, int64, int64) error { return errors.New("telegram failed") }

	svc.ProcessExpiredSubscriptions(ctx)

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
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokeFailed); got != 1 {
		t.Fatalf("subscription_revoke_failed count = %d, want 1", got)
	}
	if details := findAuditEventDetails(events, domain.AuditActionSubscriptionRevokeFailed); !strings.Contains(details, "attempt=1") || !strings.Contains(details, "reason=remove_chat_member_failed") {
		t.Fatalf("subscription_revoke_failed details=%q want attempt=1 remove_chat_member_failed", details)
	}
}

func TestProcessExpiredSubscriptions_WithoutTelegramClient_WritesRevokeFailureAudit(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()

	connectorID := seedConnector(t, ctx, st, "in-lifecycle-expire-no-tg-client")
	subscriptionID := seedActiveSubscription(t, ctx, st, connectorID, 880204, "sub-expire-no-tg-client", time.Now().UTC().Add(-1*time.Minute))
	appCtx := &application{
		config: config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:  st,
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
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokedFromChat); got != 0 {
		t.Fatalf("subscription_revoked_from_chat count = %d, want 0", got)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokeFailed); got != 1 {
		t.Fatalf("subscription_revoke_failed count = %d, want 1", got)
	}
	if details := findAuditEventDetails(events, domain.AuditActionSubscriptionRevokeFailed); !strings.Contains(details, "reason=telegram_client_not_configured") {
		t.Fatalf("subscription_revoke_failed details=%q want telegram_client_not_configured", details)
	}
}

func TestProcessFailedSubscriptionRevokes_SkipsRetryWhenOtherConnectorKeepsSameTelegramChatAccess(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()

	oldConnectorID := seedConnector(t, ctx, st, "retry-same-chat-old")
	newConnectorID := seedConnector(t, ctx, st, "retry-same-chat-new")
	userID := seedTelegramUser(t, ctx, st, 880205)
	oldSubscriptionID := seedActiveSubscriptionForUser(t, ctx, st, oldConnectorID, userID, 880205, "sub-retry-same-chat-old", time.Now().UTC().Add(-1*time.Hour))
	seedActiveSubscriptionForUser(t, ctx, st, newConnectorID, userID, 880205, "sub-retry-same-chat-new", time.Now().UTC().Add(29*24*time.Hour))
	if err := st.UpdateSubscriptionStatus(ctx, oldSubscriptionID, domain.SubscriptionStatusExpired, time.Now().UTC()); err != nil {
		t.Fatalf("expire old subscription: %v", err)
	}
	if err := st.SaveAuditEvent(ctx, domain.AuditEvent{
		ActorType:    domain.AuditActorTypeApp,
		TargetUserID: userID,
		ConnectorID:  oldConnectorID,
		Action:       domain.AuditActionSubscriptionRevokeFailed,
		Details:      "subscription_id=" + strconv.FormatInt(oldSubscriptionID, 10) + ";attempt=1;source=expiry_job;reason=remove_chat_member_failed",
		CreatedAt:    time.Now().UTC().Add(-10 * time.Minute),
	}); err != nil {
		t.Fatalf("save audit event: %v", err)
	}

	appCtx := &application{
		config: config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:  st,
	}
	svc := appCtx.subscriptionLifecycleService()
	revokeCalls := 0
	svc.RemoveTelegramChatMember = func(context.Context, int64, int64) error {
		revokeCalls++
		return nil
	}

	svc.ProcessFailedSubscriptionRevokes(ctx)

	if revokeCalls != 0 {
		t.Fatalf("revoke calls = %d, want 0", revokeCalls)
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokeFailed); got != 1 {
		t.Fatalf("subscription_revoke_failed count = %d, want 1", got)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokedFromChat); got != 0 {
		t.Fatalf("subscription_revoked_from_chat count = %d, want 0", got)
	}
}

func TestProcessSubscriptionRevokeRetries_MarksManualCheckAfterRetryExhaustion(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}

	connectorID := seedConnector(t, ctx, st, "in-lifecycle-revoke-retries")
	subscriptionID := seedActiveSubscription(t, ctx, st, connectorID, 880204, "sub-expire-retries", time.Now().UTC().Add(-2*time.Hour))
	appCtx := &application{
		config:         config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:          st,
		telegramClient: tg,
	}
	if err := st.UpdateSubscriptionStatus(ctx, subscriptionID, domain.SubscriptionStatusExpired, time.Now().UTC().Add(-90*time.Minute)); err != nil {
		t.Fatalf("UpdateSubscriptionStatus err=%v", err)
	}
	sub, found, err := st.GetSubscriptionByID(ctx, subscriptionID)
	if err != nil || !found {
		t.Fatalf("GetSubscriptionByID found=%v err=%v", found, err)
	}

	failureAtOne := time.Now().UTC().Add(-2 * time.Hour)
	failureAtTwo := time.Now().UTC().Add(-40 * time.Minute)
	if err := st.SaveAuditEvent(ctx, appCtx.buildAppTargetAuditEvent(ctx, sub.UserID, "", sub.ConnectorID, domain.AuditActionSubscriptionRevokeFailed, "subscription_id="+strconv.FormatInt(sub.ID, 10)+";attempt=1;source=expiry_job;reason=remove_chat_member_failed", failureAtOne)); err != nil {
		t.Fatalf("SaveAuditEvent attempt1 err=%v", err)
	}
	if err := st.SaveAuditEvent(ctx, appCtx.buildAppTargetAuditEvent(ctx, sub.UserID, "", sub.ConnectorID, domain.AuditActionSubscriptionRevokeFailed, "subscription_id="+strconv.FormatInt(sub.ID, 10)+";attempt=2;source=retry_job;reason=remove_chat_member_failed", failureAtTwo)); err != nil {
		t.Fatalf("SaveAuditEvent attempt2 err=%v", err)
	}

	svc := appCtx.subscriptionLifecycleService()
	svc.RemoveTelegramChatMember = func(context.Context, int64, int64) error { return errors.New("telegram failed again") }
	svc.ProcessFailedSubscriptionRevokes(ctx)

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokeFailed); got != 3 {
		t.Fatalf("subscription_revoke_failed count = %d, want 3", got)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokeManualCheck); got != 1 {
		t.Fatalf("subscription_revoke_manual_check_required count = %d, want 1", got)
	}
	if details := findAuditEventDetails(events, domain.AuditActionSubscriptionRevokeManualCheck); !strings.Contains(details, "failed_attempts=3") {
		t.Fatalf("subscription_revoke_manual_check details=%q want failed_attempts=3", details)
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

func TestProcessSubscriptionExpiryNotices_SkipsFutureQueuedSubscription(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}

	now := time.Now().UTC()
	connectorID := seedConnector(t, ctx, st, "in-lifecycle-expiry-notice-future")
	userID := seedTelegramUser(t, ctx, st, 880012)
	seedPayment(t, ctx, st, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPaid,
		Token:       "sub-expiry-notice-future",
		UserID:      userID,
		ConnectorID: connectorID,
		AmountRUB:   2322,
		CreatedAt:   now.Add(-time.Hour),
		UpdatedAt:   now.Add(-time.Hour),
		PaidAt:      &now,
	})
	paymentRow, found, err := st.GetPaymentByToken(ctx, "sub-expiry-notice-future")
	if err != nil || !found {
		t.Fatalf("get payment by token: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:      userID,
		ConnectorID: connectorID,
		PaymentID:   paymentRow.ID,
		Status:      domain.SubscriptionStatusActive,
		StartsAt:    now.Add(2 * time.Hour),
		EndsAt:      now.Add(12 * time.Hour),
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("upsert subscription: %v", err)
	}

	appCtx := &application{
		config:         config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:          st,
		telegramClient: tg,
	}

	processSubscriptionExpiryNotices(ctx, appCtx)

	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, userID, connectorID)
	if err != nil || !found {
		t.Fatalf("get latest subscription: found=%v err=%v", found, err)
	}
	if sub.ExpiryNoticeSentAt != nil {
		t.Fatalf("expiry_notice_sent_at = %v, want nil", sub.ExpiryNoticeSentAt)
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionExpiryNoticeSent); got != 0 {
		t.Fatalf("subscription_expiry_notice_sent count = %d, want 0", got)
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

func TestProcessExpiredSubscriptions_MAXRevokesMemberWhenChatIDConfigured(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	maxSpy := &spySender{}
	now := time.Now().UTC()

	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465779", "Федор Николаевич")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}
	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-expired-max-revoke",
		Name:          "max revoke",
		MAXChatID:     "-72598909498032",
		MAXChannelURL: "https://web.max.ru/-72598909498032",
		PriceRUB:      1,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-expired-max-revoke")
	if err != nil || !found {
		t.Fatalf("get connector found=%v err=%v", found, err)
	}
	seedActiveSubscriptionForUser(t, ctx, st, connector.ID, maxUser.ID, 193465779, "sub-max-expired-revoke", now.Add(-1*time.Minute))

	appCtx := &application{
		config:    config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:     st,
		maxSender: maxSpy,
	}
	svc := appCtx.subscriptionLifecycleService()
	removeCalls := 0
	var removedChatID, removedUserID int64
	svc.RemoveMAXChatMember = func(_ context.Context, chatID, userID int64) error {
		removeCalls++
		removedChatID = chatID
		removedUserID = userID
		return nil
	}

	svc.ProcessExpiredSubscriptions(ctx)

	if removeCalls != 1 {
		t.Fatalf("remove calls = %d, want 1", removeCalls)
	}
	if removedChatID != -72598909498032 || removedUserID != 193465779 {
		t.Fatalf("removed chat/user = (%d,%d) want (-72598909498032,193465779)", removedChatID, removedUserID)
	}
	if len(maxSpy.sent) != 1 {
		t.Fatalf("max sent messages = %d, want 1", len(maxSpy.sent))
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokedFromChat); got != 1 {
		t.Fatalf("subscription_revoked_from_chat count = %d, want 1", got)
	}
}

func TestProcessExpiredSubscriptions_MAXOnlyConnectorIgnoresTelegramPreferredFallback(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	maxSpy := &spySender{}
	now := time.Now().UTC()

	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465781", "Федор Николаевич")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}
	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-expired-max-fallback",
		Name:          "max revoke fallback",
		MAXChatID:     "-72598909498032",
		MAXChannelURL: "https://web.max.ru/-72598909498032",
		PriceRUB:      1,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-expired-max-fallback")
	if err != nil || !found {
		t.Fatalf("get connector found=%v err=%v", found, err)
	}
	seedActiveSubscriptionForUser(t, ctx, st, connector.ID, maxUser.ID, 193465781, "sub-max-fallback", now.Add(-1*time.Minute))

	appCtx := &application{
		config:    config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:     st,
		maxSender: maxSpy,
	}
	svc := appCtx.subscriptionLifecycleService()
	svc.ResolvePreferredKind = func(context.Context, int64, string) messenger.Kind {
		return messenger.KindTelegram
	}
	removeCalls := 0
	svc.RemoveMAXChatMember = func(_ context.Context, chatID, userID int64) error {
		removeCalls++
		if chatID != -72598909498032 || userID != 193465781 {
			t.Fatalf("removed chat/user = (%d,%d) want (-72598909498032,193465781)", chatID, userID)
		}
		return nil
	}

	svc.ProcessExpiredSubscriptions(ctx)

	if removeCalls != 1 {
		t.Fatalf("remove calls = %d, want 1", removeCalls)
	}
}

func TestProcessExpiredSubscriptions_MAXSkipsRevokeWhenOtherConnectorKeepsSameChatAccess(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	maxSpy := &spySender{}
	now := time.Now().UTC()

	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465780", "Федор Николаевич")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}
	oldConnector := domain.Connector{
		StartPayload:  "in-expired-max-same-chat-old",
		Name:          "max old",
		MAXChatID:     "-72598909498032",
		MAXChannelURL: "https://web.max.ru/-72598909498032",
		PriceRUB:      1,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}
	newConnector := oldConnector
	newConnector.StartPayload = "in-expired-max-same-chat-new"
	newConnector.Name = "max new"
	if err := st.CreateConnector(ctx, oldConnector); err != nil {
		t.Fatalf("create old connector: %v", err)
	}
	if err := st.CreateConnector(ctx, newConnector); err != nil {
		t.Fatalf("create new connector: %v", err)
	}
	oldLoaded, found, err := st.GetConnectorByStartPayload(ctx, oldConnector.StartPayload)
	if err != nil || !found {
		t.Fatalf("get old connector found=%v err=%v", found, err)
	}
	newLoaded, found, err := st.GetConnectorByStartPayload(ctx, newConnector.StartPayload)
	if err != nil || !found {
		t.Fatalf("get new connector found=%v err=%v", found, err)
	}
	seedActiveSubscriptionForUser(t, ctx, st, oldLoaded.ID, maxUser.ID, 193465780, "sub-max-same-chat-old", now.Add(-1*time.Minute))
	seedActiveSubscriptionForUser(t, ctx, st, newLoaded.ID, maxUser.ID, 193465780, "sub-max-same-chat-new", now.Add(29*24*time.Hour))

	appCtx := &application{
		config:    config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:     st,
		maxSender: maxSpy,
	}
	svc := appCtx.subscriptionLifecycleService()
	removeCalls := 0
	svc.RemoveMAXChatMember = func(context.Context, int64, int64) error {
		removeCalls++
		return nil
	}

	svc.ProcessExpiredSubscriptions(ctx)

	if removeCalls != 0 {
		t.Fatalf("remove calls = %d, want 0", removeCalls)
	}
	if len(maxSpy.sent) != 0 {
		t.Fatalf("max sent messages = %d, want 0", len(maxSpy.sent))
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if details := findAuditEventDetails(events, domain.AuditActionSubscriptionExpired); !strings.Contains(details, "same_max_chat_active=true") {
		t.Fatalf("subscription_expired details=%q want same_max_chat_active=true", details)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokedFromChat); got != 0 {
		t.Fatalf("subscription_revoked_from_chat count = %d, want 0", got)
	}
}

func TestProcessSubscriptionRevokeRetries_MAXMarksManualCheckAfterRetryExhaustion(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	now := time.Now().UTC()

	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465782", "Федор Николаевич")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}
	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-max-revoke-retries",
		Name:          "max revoke retries",
		MAXChatID:     "-72598909498032",
		MAXChannelURL: "https://web.max.ru/-72598909498032",
		PriceRUB:      1,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-max-revoke-retries")
	if err != nil || !found {
		t.Fatalf("get connector found=%v err=%v", found, err)
	}
	seedActiveSubscriptionForUser(t, ctx, st, connector.ID, maxUser.ID, 193465782, "sub-max-retry", now.Add(-2*time.Hour))

	appCtx := &application{
		config:    config.Config{Telegram: config.TelegramConfig{BotUsername: "test_bot"}},
		store:     st,
		maxSender: &spySender{},
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, maxUser.ID, connector.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}
	if err := st.UpdateSubscriptionStatus(ctx, sub.ID, domain.SubscriptionStatusExpired, now.Add(-90*time.Minute)); err != nil {
		t.Fatalf("UpdateSubscriptionStatus err=%v", err)
	}
	sub, found, err = st.GetSubscriptionByID(ctx, sub.ID)
	if err != nil || !found {
		t.Fatalf("GetSubscriptionByID found=%v err=%v", found, err)
	}

	failureAtOne := now.Add(-2 * time.Hour)
	failureAtTwo := now.Add(-40 * time.Minute)
	if err := st.SaveAuditEvent(ctx, appCtx.buildAppTargetAuditEvent(ctx, sub.UserID, "", sub.ConnectorID, domain.AuditActionSubscriptionRevokeFailed, "subscription_id="+strconv.FormatInt(sub.ID, 10)+";attempt=1;source=expiry_job;reason=remove_max_chat_member_failed", failureAtOne)); err != nil {
		t.Fatalf("SaveAuditEvent attempt1 err=%v", err)
	}
	if err := st.SaveAuditEvent(ctx, appCtx.buildAppTargetAuditEvent(ctx, sub.UserID, "", sub.ConnectorID, domain.AuditActionSubscriptionRevokeFailed, "subscription_id="+strconv.FormatInt(sub.ID, 10)+";attempt=2;source=retry_job;reason=remove_max_chat_member_failed", failureAtTwo)); err != nil {
		t.Fatalf("SaveAuditEvent attempt2 err=%v", err)
	}

	svc := appCtx.subscriptionLifecycleService()
	svc.RemoveMAXChatMember = func(context.Context, int64, int64) error { return errors.New("max failed again") }
	svc.ProcessFailedSubscriptionRevokes(ctx)

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokeFailed); got != 3 {
		t.Fatalf("subscription_revoke_failed count = %d, want 3", got)
	}
	if got := countAuditEvents(events, domain.AuditActionSubscriptionRevokeManualCheck); got != 1 {
		t.Fatalf("subscription_revoke_manual_check count = %d, want 1", got)
	}
	if details := findAuditEventDetails(events, domain.AuditActionSubscriptionRevokeManualCheck); !strings.Contains(details, "failed_attempts=3") {
		t.Fatalf("subscription_revoke_manual_check details=%q want failed_attempts=3", details)
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

func findAuditEventDetails(events []domain.AuditEvent, action string) string {
	for _, event := range events {
		if event.Action == action {
			return event.Details
		}
	}
	return ""
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
