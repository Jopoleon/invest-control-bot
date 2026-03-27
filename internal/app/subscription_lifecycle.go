package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/go-co-op/gocron/v2"
)

const (
	reminderDaysBeforeEnd = 3
	expiryNoticeWindow    = 24 * time.Hour
	subscriptionJobLimit  = 200
	subscriptionJobEvery  = time.Minute
)

// newSubscriptionLifecycleScheduler wires periodic reminder and expiration jobs.
func newSubscriptionLifecycleScheduler(appCtx *application) (gocron.Scheduler, error) {
	scheduler, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("create subscription lifecycle scheduler: %w", err)
	}

	jobOptions := []gocron.JobOption{
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	}
	if _, err := scheduler.NewJob(
		gocron.DurationJob(subscriptionJobEvery),
		gocron.NewTask(func() {
			processSubscriptionReminders(context.Background(), appCtx)
		}),
		jobOptions...,
	); err != nil {
		_ = scheduler.Shutdown()
		return nil, fmt.Errorf("register subscription reminder job: %w", err)
	}
	if _, err := scheduler.NewJob(
		gocron.DurationJob(subscriptionJobEvery),
		gocron.NewTask(func() {
			processSubscriptionExpiryNotices(context.Background(), appCtx)
		}),
		jobOptions...,
	); err != nil {
		_ = scheduler.Shutdown()
		return nil, fmt.Errorf("register subscription expiry notice job: %w", err)
	}
	if _, err := scheduler.NewJob(
		gocron.DurationJob(subscriptionJobEvery),
		gocron.NewTask(func() {
			processExpiredSubscriptions(context.Background(), appCtx)
		}),
		jobOptions...,
	); err != nil {
		_ = scheduler.Shutdown()
		return nil, fmt.Errorf("register subscription expiration job: %w", err)
	}
	if appCtx.robokassaService != nil && appCtx.config.Payment.Robokassa.RecurringEnabled {
		if _, err := scheduler.NewJob(
			gocron.DurationJob(subscriptionJobEvery),
			gocron.NewTask(func() {
				processRecurringRebills(context.Background(), appCtx)
			}),
			jobOptions...,
		); err != nil {
			_ = scheduler.Shutdown()
			return nil, fmt.Errorf("register recurring rebill job: %w", err)
		}
	}

	return scheduler, nil
}

// runSubscriptionLifecyclePass executes both lifecycle phases once.
func runSubscriptionLifecyclePass(ctx context.Context, appCtx *application) {
	processSubscriptionReminders(ctx, appCtx)
	processSubscriptionExpiryNotices(ctx, appCtx)
	if appCtx.robokassaService != nil && appCtx.config.Payment.Robokassa.RecurringEnabled {
		processRecurringRebills(ctx, appCtx)
	}
	processExpiredSubscriptions(ctx, appCtx)
}

func processRecurringRebills(ctx context.Context, appCtx *application) {
	subs, err := appCtx.store.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		Status: domain.SubscriptionStatusActive,
		Limit:  subscriptionJobLimit,
	})
	if err != nil {
		slog.Error("list subscriptions for recurring rebill failed", "error", err)
		return
	}

	now := time.Now().UTC()
	for _, sub := range subs {
		if !sub.AutoPayEnabled {
			continue
		}
		shouldTrigger, err := appCtx.shouldTriggerScheduledRebill(ctx, sub, now)
		if err != nil {
			slog.Error("check scheduled rebill eligibility failed", "error", err, "subscription_id", sub.ID)
			continue
		}
		if !shouldTrigger {
			continue
		}
		if _, err := appCtx.triggerRebill(ctx, sub.ID, "scheduler"); err != nil {
			if errors.Is(err, errRebillRequestFailed) {
				slog.Warn("scheduled rebill request failed", "subscription_id", sub.ID, "error", err)
				continue
			}
			slog.Error("scheduled rebill failed", "error", err, "subscription_id", sub.ID)
		}
	}
}

func processSubscriptionReminders(ctx context.Context, appCtx *application) {
	now := time.Now().UTC()
	remindBefore := now.AddDate(0, 0, reminderDaysBeforeEnd)
	subs, err := appCtx.store.ListSubscriptionsForReminder(ctx, remindBefore, subscriptionJobLimit)
	if err != nil {
		slog.Error("list reminder due subscriptions failed", "error", err)
		return
	}
	for _, sub := range subs {
		connector, ok, err := appCtx.store.GetConnector(ctx, sub.ConnectorID)
		if err != nil {
			slog.Error("load connector for reminder failed", "error", err, "subscription_id", sub.ID, "connector_id", sub.ConnectorID)
			continue
		}
		if !ok {
			continue
		}

		text := appSubscriptionReminderMessage(sub.EndsAt)
		msg := appCtx.buildRenewalNotification(ctx, sub.UserID, sub.TelegramID, connector.StartPayload, text)
		if err := appCtx.sendUserNotification(ctx, sub.UserID, sub.TelegramID, msg); err != nil {
			slog.Error("send subscription reminder failed", "error", err, "subscription_id", sub.ID, "user_id", sub.UserID, "legacy_external_id", sub.TelegramID)
			continue
		}
		if err := appCtx.store.MarkSubscriptionReminderSent(ctx, sub.ID, now); err != nil {
			slog.Error("mark subscription reminder sent failed", "error", err, "subscription_id", sub.ID)
		}
		_ = appCtx.store.SaveAuditEvent(ctx, domain.AuditEvent{
			TelegramID:  sub.TelegramID,
			ConnectorID: sub.ConnectorID,
			Action:      domain.AuditActionSubscriptionReminderSent,
			Details:     "subscription_id=" + strconv.FormatInt(sub.ID, 10),
			CreatedAt:   now,
		})
	}
}

func processSubscriptionExpiryNotices(ctx context.Context, appCtx *application) {
	now := time.Now().UTC()
	noticeBefore := now.Add(expiryNoticeWindow)
	subs, err := appCtx.store.ListSubscriptionsForExpiryNotice(ctx, noticeBefore, subscriptionJobLimit)
	if err != nil {
		slog.Error("list expiry notice subscriptions failed", "error", err)
		return
	}
	for _, sub := range subs {
		connector, ok, err := appCtx.store.GetConnector(ctx, sub.ConnectorID)
		if err != nil {
			slog.Error("load connector for expiry notice failed", "error", err, "subscription_id", sub.ID, "connector_id", sub.ConnectorID)
			continue
		}
		if !ok {
			continue
		}

		text := appSubscriptionExpiryNoticeMessage(sub.EndsAt)
		msg := appCtx.buildRenewalNotification(ctx, sub.UserID, sub.TelegramID, connector.StartPayload, text)
		if err := appCtx.sendUserNotification(ctx, sub.UserID, sub.TelegramID, msg); err != nil {
			slog.Error("send subscription expiry notice failed", "error", err, "subscription_id", sub.ID, "user_id", sub.UserID, "legacy_external_id", sub.TelegramID)
			continue
		}
		if err := appCtx.store.MarkSubscriptionExpiryNoticeSent(ctx, sub.ID, now); err != nil {
			slog.Error("mark subscription expiry notice sent failed", "error", err, "subscription_id", sub.ID)
		}
		_ = appCtx.store.SaveAuditEvent(ctx, domain.AuditEvent{
			TelegramID:  sub.TelegramID,
			ConnectorID: sub.ConnectorID,
			Action:      domain.AuditActionSubscriptionExpiryNoticeSent,
			Details:     "subscription_id=" + strconv.FormatInt(sub.ID, 10),
			CreatedAt:   now,
		})
	}
}

func processExpiredSubscriptions(ctx context.Context, appCtx *application) {
	now := time.Now().UTC()
	subs, err := appCtx.store.ListExpiredActiveSubscriptions(ctx, now, subscriptionJobLimit)
	if err != nil {
		slog.Error("list expired subscriptions failed", "error", err)
		return
	}
	for _, sub := range subs {
		connector, connectorFound, err := appCtx.store.GetConnector(ctx, sub.ConnectorID)
		if err != nil {
			slog.Error("load connector for expiration failed", "error", err, "subscription_id", sub.ID, "connector_id", sub.ConnectorID)
		}

		// Business status transition to expired.
		if err := appCtx.store.UpdateSubscriptionStatus(ctx, sub.ID, domain.SubscriptionStatusExpired, now); err != nil {
			slog.Error("update subscription status failed", "error", err, "subscription_id", sub.ID)
			continue
		}

		// Best-effort revoke from chat when chat_id is configured and bot has rights.
		if connectorFound && appCtx.resolvePreferredMessengerKind(ctx, sub.UserID, sub.TelegramID) == messenger.KindTelegram {
			if chatID, ok := normalizeTelegramChatID(connector.ChatID); ok {
				if err := appCtx.telegramClient.RemoveChatMember(ctx, chatID, sub.TelegramID); err != nil {
					slog.Error("remove chat member failed", "error", err, "subscription_id", sub.ID, "telegram_id", sub.TelegramID, "chat_id", chatID)
					_ = appCtx.store.SaveAuditEvent(ctx, domain.AuditEvent{
						TelegramID:  sub.TelegramID,
						ConnectorID: sub.ConnectorID,
						Action:      domain.AuditActionSubscriptionRevokeFailed,
						Details:     "subscription_id=" + strconv.FormatInt(sub.ID, 10),
						CreatedAt:   now,
					})
				} else {
					_ = appCtx.store.SaveAuditEvent(ctx, domain.AuditEvent{
						TelegramID:  sub.TelegramID,
						ConnectorID: sub.ConnectorID,
						Action:      domain.AuditActionSubscriptionRevokedFromChat,
						Details:     "subscription_id=" + strconv.FormatInt(sub.ID, 10),
						CreatedAt:   now,
					})
				}
			}
		}

		text := appSubscriptionExpiredText
		msg := messenger.OutgoingMessage{Text: text}
		if connectorFound {
			msg = appCtx.buildRenewalNotification(ctx, sub.UserID, sub.TelegramID, connector.StartPayload, text)
		}
		if err := appCtx.sendUserNotification(ctx, sub.UserID, sub.TelegramID, msg); err != nil {
			slog.Error("send subscription expired message failed", "error", err, "subscription_id", sub.ID, "user_id", sub.UserID, "legacy_external_id", sub.TelegramID)
		}
		_ = appCtx.store.SaveAuditEvent(ctx, domain.AuditEvent{
			TelegramID:  sub.TelegramID,
			ConnectorID: sub.ConnectorID,
			Action:      domain.AuditActionSubscriptionExpired,
			Details:     "subscription_id=" + strconv.FormatInt(sub.ID, 10),
			CreatedAt:   now,
		})
	}
}

func buildBotStartURL(botUsername, startPayload string) string {
	username := strings.TrimSpace(strings.TrimPrefix(botUsername, "@"))
	payload := strings.TrimSpace(startPayload)
	if username == "" || payload == "" {
		return ""
	}
	return "https://t.me/" + username + "?start=" + payload
}

func buildBotStartCommand(startPayload string) string {
	payload := strings.TrimSpace(startPayload)
	if payload == "" {
		return ""
	}
	return "/start " + payload
}

func (a *application) buildRenewalNotification(ctx context.Context, userID, legacyExternalID int64, startPayload, text string) messenger.OutgoingMessage {
	msg := messenger.OutgoingMessage{Text: text}
	payload := strings.TrimSpace(startPayload)
	if payload == "" {
		return msg
	}

	switch a.resolvePreferredMessengerKind(ctx, userID, legacyExternalID) {
	case messenger.KindMAX:
		msg.Text += fmt.Sprintf(appSubscriptionReminderCommandFmt, payload)
	default:
		if renewURL := buildBotStartURL(a.config.Telegram.BotUsername, payload); renewURL != "" {
			msg.Buttons = [][]messenger.ActionButton{{
				{Text: appSubscriptionRenewButton, URL: renewURL},
			}}
		}
	}

	return msg
}

func normalizeTelegramChatID(chatIDRaw string) (int64, bool) {
	raw := strings.TrimSpace(chatIDRaw)
	if raw == "" {
		return 0, false
	}
	// Admin UI stores chat IDs without minus; convert to Telegram expected negative IDs.
	raw = strings.TrimPrefix(raw, "+")
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value == 0 {
		return 0, false
	}
	if value < 0 {
		return value, true
	}
	return -value, true
}
