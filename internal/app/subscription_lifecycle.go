package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	appsubscriptions "github.com/Jopoleon/invest-control-bot/internal/app/subscriptions"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/go-co-op/gocron/v2"
)

const (
	reminderDaysBeforeEnd = 3
	expiryNoticeWindow    = 24 * time.Hour
	subscriptionJobLimit  = 200
	// Short smoke-test periods use an alternate strategy in app/subscriptions
	// and app/recurring. The cadence stays below one minute so 60s/120s test
	// periods have a realistic chance to hit their rebill windows.
	subscriptionJobEvery = 10 * time.Second
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
			processSubscriptionRevokeRetries(context.Background(), appCtx)
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
	processSubscriptionRevokeRetries(ctx, appCtx)
}

func processRecurringRebills(ctx context.Context, appCtx *application) {
	subs, err := appCtx.store.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		Status: domain.SubscriptionStatusActive,
		Limit:  subscriptionJobLimit,
	})
	if err != nil {
		return
	}

	now := time.Now().UTC()
	for _, sub := range subs {
		if !sub.AutoPayEnabled {
			continue
		}
		decision, err := appCtx.evaluateScheduledRebill(ctx, sub, now)
		if err != nil {
			continue
		}
		if decision.ShortDuration {
			slog.Info("short-period rebill scheduler decision",
				"subscription_id", sub.ID,
				"user_id", sub.UserID,
				"connector_id", sub.ConnectorID,
				"remaining", decision.Remaining,
				"target_attempt", decision.TargetAttempt,
				"failed_attempts", decision.FailedAttempts,
				"reason", decision.Reason,
				"trigger", decision.Trigger,
			)
		}
		if decision.PendingPayment != nil {
			appCtx.recurringService().ReportStalePendingRebill(ctx, sub, decision, now)
		}
		if !decision.Trigger {
			continue
		}
		if _, err := appCtx.triggerRebill(ctx, sub.ID, "scheduler"); err != nil {
			if errors.Is(err, errRebillRequestFailed) {
				continue
			}
		}
	}
}

func processSubscriptionReminders(ctx context.Context, appCtx *application) {
	appCtx.subscriptionLifecycleService().ProcessSubscriptionReminders(ctx)
}

func processSubscriptionExpiryNotices(ctx context.Context, appCtx *application) {
	appCtx.subscriptionLifecycleService().ProcessSubscriptionExpiryNotices(ctx)
}

func processExpiredSubscriptions(ctx context.Context, appCtx *application) {
	appCtx.subscriptionLifecycleService().ProcessExpiredSubscriptions(ctx)
}

func processSubscriptionRevokeRetries(ctx context.Context, appCtx *application) {
	appCtx.subscriptionLifecycleService().ProcessFailedSubscriptionRevokes(ctx)
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

func (a *application) buildRenewalNotification(ctx context.Context, userID int64, preferredMessengerUserID, startPayload, text string) messenger.OutgoingMessage {
	return a.subscriptionLifecycleService().BuildRenewalNotification(ctx, userID, preferredMessengerUserID, startPayload, text)
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

func (a *application) subscriptionLifecycleService() *appsubscriptions.Service {
	return &appsubscriptions.Service{
		Store:                 a.store,
		TelegramClient:        a.telegramClient,
		TelegramBotUsername:   a.config.Telegram.BotUsername,
		ReminderDaysBeforeEnd: reminderDaysBeforeEnd,
		ExpiryNoticeWindow:    expiryNoticeWindow,
		SubscriptionJobLimit:  subscriptionJobLimit,
		RemoveTelegramChatMember: func(ctx context.Context, chatID, userID int64) error {
			if a.telegramClient == nil {
				return nil
			}
			return a.telegramClient.RemoveChatMember(ctx, chatID, userID)
		},
		SubscriptionReminderMessage: appSubscriptionReminderMessage,
		SubscriptionExpiryMessage:   appSubscriptionExpiryNoticeMessage,
		SubscriptionExpiredText:     appSubscriptionExpiredText,
		RenewalButtonLabel:          appSubscriptionRenewButton,
		RenewalCommandFormat:        appSubscriptionReminderCommandFmt,
		SendUserNotification:        a.sendUserNotification,
		BuildTargetAuditEvent:       a.buildAppTargetAuditEvent,
		ResolvePreferredKind:        a.resolvePreferredMessengerKind,
		ResolveTelegramAccount:      a.resolveTelegramMessengerAccount,
	}
}
