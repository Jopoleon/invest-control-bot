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

// newSubscriptionLifecycleScheduler wires the background lifecycle loop that
// runs for the whole process lifetime.
//
// Operationally this scheduler is safe to restart during a normal deploy:
//  1. server startup runs one synchronous lifecycle pass before the ticker starts;
//  2. after that, the same jobs repeat every subscriptionJobEvery;
//  3. every job uses singleton mode, so one slow pass is rescheduled instead of
//     running concurrently with itself;
//  4. all lifecycle decisions are recomputed from the current DB state on every pass.
//
// This means a redeploy does not "resume from in-memory state" and does not
// blindly repeat previous actions. For recurring specifically, a rebill can be
// triggered only if the current subscription still passes all guards in
// recurring.Service.EvaluateScheduledRebill:
//   - subscription is active and auto_pay_enabled;
//   - subscription has already started (future queued periods are skipped);
//   - connector supports recurring;
//   - there is no pending or already-paid child rebill for this subscription;
//   - the current time is inside the connector's rebill window;
//   - failed attempt budget for the current window is not exhausted.
//
// So after the runaway fix, a clean redeploy should not recreate the old
// cascade just because the process restarted. The scheduler will see the same
// persisted subscriptions/payments and skip everything that is not genuinely due.
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

// processRecurringRebills performs one recurring scheduler sweep over the
// currently active subscriptions in storage.
//
// Important invariants:
//   - it is stateless between runs and always recomputes eligibility from DB;
//   - it only inspects active subscriptions, then immediately skips rows with
//     auto_pay_enabled=false;
//   - it does not rebill future queued subscriptions because
//     EvaluateScheduledRebill returns subscription_not_started when starts_at > now;
//   - it does not duplicate a rebill when a pending or already-paid child
//     payment exists for the same subscription;
//   - it emits observability for short-period connectors on every sweep so
//     deploy-time debugging can rely on logs instead of inferred scheduler state.
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
	// The root app package still owns transport/client wiring, while the
	// subscriptions package owns lifecycle rules. Keeping this constructor here
	// prevents Telegram/MAX clients from leaking into app/subscriptions tests.
	//
	// TODO: Unify payment, recurring, and subscription service wiring behind a
	// slimmer application composition layer. Right now root app still hand-wires
	// several service objects with overlapping dependencies.
	service := &appsubscriptions.Service{
		Store:                       a.store,
		TelegramClient:              a.telegramClient,
		TelegramBotUsername:         a.config.Telegram.BotUsername,
		ReminderDaysBeforeEnd:       reminderDaysBeforeEnd,
		ExpiryNoticeWindow:          expiryNoticeWindow,
		SubscriptionJobLimit:        subscriptionJobLimit,
		SubscriptionReminderMessage: appSubscriptionReminderMessage,
		SubscriptionExpiryMessage:   appSubscriptionExpiryNoticeMessage,
		SubscriptionExpiredText:     appSubscriptionExpiredText,
		RenewalButtonLabel:          appSubscriptionRenewButton,
		RenewalCommandFormat:        appSubscriptionReminderCommandFmt,
		SendUserNotification:        a.sendUserNotification,
		BuildTargetAuditEvent:       a.buildAppTargetAuditEvent,
		ResolvePreferredKind:        a.resolvePreferredMessengerKind,
		ResolveTelegramAccount:      a.resolveTelegramMessengerAccount,
		ResolveMAXAccount:           a.resolveMAXMessengerAccount,
	}
	if a.telegramClient != nil {
		service.RemoveTelegramChatMember = func(ctx context.Context, chatRef string, userID int64) error {
			return a.telegramClient.RemoveChatMember(ctx, chatRef, userID)
		}
		service.RevokeTelegramInviteLink = func(ctx context.Context, chatRef string, inviteLink string) error {
			return a.telegramClient.RevokeInviteLink(ctx, chatRef, inviteLink)
		}
	}
	if a.maxClient != nil {
		service.RemoveMAXChatMember = func(ctx context.Context, chatID, userID int64) error {
			return a.maxClient.RemoveChatMember(ctx, chatID, userID, false)
		}
	}
	return service
}
