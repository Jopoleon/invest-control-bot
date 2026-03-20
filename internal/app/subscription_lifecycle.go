package app

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/Jopoleon/telega-bot-fedor/internal/store"
	"github.com/Jopoleon/telega-bot-fedor/internal/telegram"
	"github.com/go-co-op/gocron/v2"
	"github.com/go-telegram/bot/models"
)

const (
	reminderDaysBeforeEnd = 3
	expiryNoticeWindow    = 24 * time.Hour
	subscriptionJobLimit  = 200
	subscriptionJobEvery  = time.Minute
)

// newSubscriptionLifecycleScheduler wires periodic reminder and expiration jobs.
func newSubscriptionLifecycleScheduler(st store.Store, tg *telegram.Client, botUsername string) (gocron.Scheduler, error) {
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
			processSubscriptionReminders(context.Background(), st, tg, botUsername)
		}),
		jobOptions...,
	); err != nil {
		_ = scheduler.Shutdown()
		return nil, fmt.Errorf("register subscription reminder job: %w", err)
	}
	if _, err := scheduler.NewJob(
		gocron.DurationJob(subscriptionJobEvery),
		gocron.NewTask(func() {
			processSubscriptionExpiryNotices(context.Background(), st, tg, botUsername)
		}),
		jobOptions...,
	); err != nil {
		_ = scheduler.Shutdown()
		return nil, fmt.Errorf("register subscription expiry notice job: %w", err)
	}
	if _, err := scheduler.NewJob(
		gocron.DurationJob(subscriptionJobEvery),
		gocron.NewTask(func() {
			processExpiredSubscriptions(context.Background(), st, tg, botUsername)
		}),
		jobOptions...,
	); err != nil {
		_ = scheduler.Shutdown()
		return nil, fmt.Errorf("register subscription expiration job: %w", err)
	}

	return scheduler, nil
}

// runSubscriptionLifecyclePass executes both lifecycle phases once.
func runSubscriptionLifecyclePass(ctx context.Context, st store.Store, tg *telegram.Client, botUsername string) {
	processSubscriptionReminders(ctx, st, tg, botUsername)
	processSubscriptionExpiryNotices(ctx, st, tg, botUsername)
	processExpiredSubscriptions(ctx, st, tg, botUsername)
}

func processSubscriptionReminders(ctx context.Context, st store.Store, tg *telegram.Client, botUsername string) {
	now := time.Now().UTC()
	remindBefore := now.AddDate(0, 0, reminderDaysBeforeEnd)
	subs, err := st.ListSubscriptionsForReminder(ctx, remindBefore, subscriptionJobLimit)
	if err != nil {
		slog.Error("list reminder due subscriptions failed", "error", err)
		return
	}
	for _, sub := range subs {
		connector, ok, err := st.GetConnector(ctx, sub.ConnectorID)
		if err != nil {
			slog.Error("load connector for reminder failed", "error", err, "subscription_id", sub.ID, "connector_id", sub.ConnectorID)
			continue
		}
		if !ok {
			continue
		}

		renewURL := buildBotStartURL(botUsername, connector.StartPayload)
		text := fmt.Sprintf("🔔 Напоминание: подписка закончится %s. Чтобы продлить доступ, нажмите кнопку ниже.",
			sub.EndsAt.In(time.Local).Format("02.01.2006 15:04"),
		)
		var keyboard *models.InlineKeyboardMarkup
		if renewURL != "" {
			keyboard = &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{{
					{Text: "Продлить подписку", URL: renewURL},
				}},
			}
		}
		if err := tg.SendMessage(ctx, sub.TelegramID, text, keyboard); err != nil {
			slog.Error("send subscription reminder failed", "error", err, "subscription_id", sub.ID, "telegram_id", sub.TelegramID)
			continue
		}
		if err := st.MarkSubscriptionReminderSent(ctx, sub.ID, now); err != nil {
			slog.Error("mark subscription reminder sent failed", "error", err, "subscription_id", sub.ID)
		}
		_ = st.SaveAuditEvent(ctx, domain.AuditEvent{
			TelegramID:  sub.TelegramID,
			ConnectorID: sub.ConnectorID,
			Action:      domain.AuditActionSubscriptionReminderSent,
			Details:     "subscription_id=" + strconv.FormatInt(sub.ID, 10),
			CreatedAt:   now,
		})
	}
}

func processSubscriptionExpiryNotices(ctx context.Context, st store.Store, tg *telegram.Client, botUsername string) {
	now := time.Now().UTC()
	noticeBefore := now.Add(expiryNoticeWindow)
	subs, err := st.ListSubscriptionsForExpiryNotice(ctx, noticeBefore, subscriptionJobLimit)
	if err != nil {
		slog.Error("list expiry notice subscriptions failed", "error", err)
		return
	}
	for _, sub := range subs {
		connector, ok, err := st.GetConnector(ctx, sub.ConnectorID)
		if err != nil {
			slog.Error("load connector for expiry notice failed", "error", err, "subscription_id", sub.ID, "connector_id", sub.ConnectorID)
			continue
		}
		if !ok {
			continue
		}

		renewURL := buildBotStartURL(botUsername, connector.StartPayload)
		text := fmt.Sprintf("⏰ Сегодня заканчивается подписка. Доступ будет отключен %s, если продление не поступит.",
			sub.EndsAt.In(time.Local).Format("02.01.2006 15:04"),
		)
		var keyboard *models.InlineKeyboardMarkup
		if renewURL != "" {
			keyboard = &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{{
					{Text: "Продлить подписку", URL: renewURL},
				}},
			}
		}
		if err := tg.SendMessage(ctx, sub.TelegramID, text, keyboard); err != nil {
			slog.Error("send subscription expiry notice failed", "error", err, "subscription_id", sub.ID, "telegram_id", sub.TelegramID)
			continue
		}
		if err := st.MarkSubscriptionExpiryNoticeSent(ctx, sub.ID, now); err != nil {
			slog.Error("mark subscription expiry notice sent failed", "error", err, "subscription_id", sub.ID)
		}
		_ = st.SaveAuditEvent(ctx, domain.AuditEvent{
			TelegramID:  sub.TelegramID,
			ConnectorID: sub.ConnectorID,
			Action:      domain.AuditActionSubscriptionExpiryNoticeSent,
			Details:     "subscription_id=" + strconv.FormatInt(sub.ID, 10),
			CreatedAt:   now,
		})
	}
}

func processExpiredSubscriptions(ctx context.Context, st store.Store, tg *telegram.Client, botUsername string) {
	now := time.Now().UTC()
	subs, err := st.ListExpiredActiveSubscriptions(ctx, now, subscriptionJobLimit)
	if err != nil {
		slog.Error("list expired subscriptions failed", "error", err)
		return
	}
	for _, sub := range subs {
		connector, connectorFound, err := st.GetConnector(ctx, sub.ConnectorID)
		if err != nil {
			slog.Error("load connector for expiration failed", "error", err, "subscription_id", sub.ID, "connector_id", sub.ConnectorID)
		}

		// Business status transition to expired.
		if err := st.UpdateSubscriptionStatus(ctx, sub.ID, domain.SubscriptionStatusExpired, now); err != nil {
			slog.Error("update subscription status failed", "error", err, "subscription_id", sub.ID)
			continue
		}

		// Best-effort revoke from chat when chat_id is configured and bot has rights.
		if connectorFound {
			if chatID, ok := normalizeTelegramChatID(connector.ChatID); ok {
				if err := tg.RemoveChatMember(ctx, chatID, sub.TelegramID); err != nil {
					slog.Error("remove chat member failed", "error", err, "subscription_id", sub.ID, "telegram_id", sub.TelegramID, "chat_id", chatID)
					_ = st.SaveAuditEvent(ctx, domain.AuditEvent{
						TelegramID:  sub.TelegramID,
						ConnectorID: sub.ConnectorID,
						Action:      domain.AuditActionSubscriptionRevokeFailed,
						Details:     "subscription_id=" + strconv.FormatInt(sub.ID, 10),
						CreatedAt:   now,
					})
				} else {
					_ = st.SaveAuditEvent(ctx, domain.AuditEvent{
						TelegramID:  sub.TelegramID,
						ConnectorID: sub.ConnectorID,
						Action:      domain.AuditActionSubscriptionRevokedFromChat,
						Details:     "subscription_id=" + strconv.FormatInt(sub.ID, 10),
						CreatedAt:   now,
					})
				}
			}
		}

		renewURL := ""
		if connectorFound {
			renewURL = buildBotStartURL(botUsername, connector.StartPayload)
		}
		text := "⏰ Срок подписки истек. Чтобы восстановить доступ, оформите продление."
		var keyboard *models.InlineKeyboardMarkup
		if renewURL != "" {
			keyboard = &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{{
					{Text: "Продлить подписку", URL: renewURL},
				}},
			}
		}
		if err := tg.SendMessage(ctx, sub.TelegramID, text, keyboard); err != nil {
			slog.Error("send subscription expired message failed", "error", err, "subscription_id", sub.ID, "telegram_id", sub.TelegramID)
		}
		_ = st.SaveAuditEvent(ctx, domain.AuditEvent{
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
