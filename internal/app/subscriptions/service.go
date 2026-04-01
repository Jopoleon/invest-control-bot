package subscriptions

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
)

// Service owns subscription lifecycle use cases at the app layer.
//
// The root app package injects the few dependencies that still cross package
// boundaries: messenger delivery, audit-event creation, and identity
// resolution. This keeps the lifecycle rules testable without depending on the
// whole application struct.
type Service struct {
	Store                       store.Store
	TelegramClient              *telegram.Client
	TelegramBotUsername         string
	ReminderDaysBeforeEnd       int
	ExpiryNoticeWindow          time.Duration
	SubscriptionJobLimit        int
	SubscriptionReminderMessage func(time.Time) string
	SubscriptionExpiryMessage   func(time.Time) string
	SubscriptionExpiredText     string
	RenewalButtonLabel          string
	RenewalCommandFormat        string
	SendUserNotification        func(context.Context, int64, string, messenger.OutgoingMessage) error
	BuildTargetAuditEvent       func(context.Context, int64, string, int64, string, string, time.Time) domain.AuditEvent
	ResolvePreferredKind        func(context.Context, int64, string) messenger.Kind
	ResolveTelegramAccount      func(context.Context, int64) (domain.UserMessengerAccount, bool, error)
}

func (s *Service) ProcessSubscriptionReminders(ctx context.Context) {
	now := time.Now().UTC()
	remindBefore := now.AddDate(0, 0, s.ReminderDaysBeforeEnd)
	subs, err := s.Store.ListSubscriptionsForReminder(ctx, remindBefore, s.SubscriptionJobLimit)
	if err != nil {
		slog.Error("list reminder due subscriptions failed", "error", err)
		return
	}
	for _, sub := range subs {
		connector, ok, err := s.Store.GetConnector(ctx, sub.ConnectorID)
		if err != nil {
			slog.Error("load connector for reminder failed", "error", err, "subscription_id", sub.ID, "connector_id", sub.ConnectorID)
			continue
		}
		if !ok {
			continue
		}
		if !shouldSendReminder(now, sub.EndsAt, connector) {
			continue
		}

		text := s.SubscriptionReminderMessage(sub.EndsAt)
		preferredMessengerUserID := ""
		msg := s.BuildRenewalNotification(ctx, sub.UserID, preferredMessengerUserID, connector.StartPayload, text)
		if err := s.SendUserNotification(ctx, sub.UserID, preferredMessengerUserID, msg); err != nil {
			slog.Error("send subscription reminder failed", "error", err, "subscription_id", sub.ID, "user_id", sub.UserID, "preferred_messenger_user_id", preferredMessengerUserID)
			continue
		}
		if err := s.Store.MarkSubscriptionReminderSent(ctx, sub.ID, now); err != nil {
			slog.Error("mark subscription reminder sent failed", "error", err, "subscription_id", sub.ID)
		}
		_ = s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(ctx, sub.UserID, preferredMessengerUserID, sub.ConnectorID, domain.AuditActionSubscriptionReminderSent, "subscription_id="+strconv.FormatInt(sub.ID, 10), now))
	}
}

func (s *Service) ProcessSubscriptionExpiryNotices(ctx context.Context) {
	now := time.Now().UTC()
	noticeBefore := now.Add(s.ExpiryNoticeWindow)
	subs, err := s.Store.ListSubscriptionsForExpiryNotice(ctx, noticeBefore, s.SubscriptionJobLimit)
	if err != nil {
		slog.Error("list expiry notice subscriptions failed", "error", err)
		return
	}
	for _, sub := range subs {
		connector, ok, err := s.Store.GetConnector(ctx, sub.ConnectorID)
		if err != nil {
			slog.Error("load connector for expiry notice failed", "error", err, "subscription_id", sub.ID, "connector_id", sub.ConnectorID)
			continue
		}
		if !ok {
			continue
		}
		if !shouldSendExpiryNotice(connector) {
			continue
		}

		text := s.SubscriptionExpiryMessage(sub.EndsAt)
		preferredMessengerUserID := ""
		msg := s.BuildRenewalNotification(ctx, sub.UserID, preferredMessengerUserID, connector.StartPayload, text)
		if err := s.SendUserNotification(ctx, sub.UserID, preferredMessengerUserID, msg); err != nil {
			slog.Error("send subscription expiry notice failed", "error", err, "subscription_id", sub.ID, "user_id", sub.UserID, "preferred_messenger_user_id", preferredMessengerUserID)
			continue
		}
		if err := s.Store.MarkSubscriptionExpiryNoticeSent(ctx, sub.ID, now); err != nil {
			slog.Error("mark subscription expiry notice sent failed", "error", err, "subscription_id", sub.ID)
		}
		_ = s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(ctx, sub.UserID, preferredMessengerUserID, sub.ConnectorID, domain.AuditActionSubscriptionExpiryNoticeSent, "subscription_id="+strconv.FormatInt(sub.ID, 10), now))
	}
}

func (s *Service) ProcessExpiredSubscriptions(ctx context.Context) {
	now := time.Now().UTC()
	subs, err := s.Store.ListExpiredActiveSubscriptions(ctx, now, s.SubscriptionJobLimit)
	if err != nil {
		slog.Error("list expired subscriptions failed", "error", err)
		return
	}
	for _, sub := range subs {
		connector, connectorFound, err := s.Store.GetConnector(ctx, sub.ConnectorID)
		if err != nil {
			slog.Error("load connector for expiration failed", "error", err, "subscription_id", sub.ID, "connector_id", sub.ConnectorID)
		}

		// Business status transition to expired.
		if err := s.Store.UpdateSubscriptionStatus(ctx, sub.ID, domain.SubscriptionStatusExpired, now); err != nil {
			slog.Error("update subscription status failed", "error", err, "subscription_id", sub.ID)
			continue
		}

		// Best-effort revoke from chat when chat_id is configured and the user's
		// preferred delivery path is Telegram. MAX does not use chat-member revoke.
		preferredMessengerUserID := ""
		if connectorFound && s.ResolvePreferredKind(ctx, sub.UserID, preferredMessengerUserID) == messenger.KindTelegram {
			if chatID, ok := normalizeTelegramChatID(connector.ChatID); ok {
				account, found, err := s.ResolveTelegramAccount(ctx, sub.UserID)
				if err != nil {
					slog.Error("resolve telegram account for revoke failed", "error", err, "subscription_id", sub.ID, "user_id", sub.UserID)
				} else if found {
					telegramID, parseErr := strconv.ParseInt(account.MessengerUserID, 10, 64)
					if parseErr != nil || telegramID <= 0 {
						slog.Error("invalid telegram account id for revoke", "error", parseErr, "subscription_id", sub.ID, "user_id", sub.UserID, "messenger_user_id", account.MessengerUserID)
					} else if err := s.TelegramClient.RemoveChatMember(ctx, chatID, telegramID); err != nil {
						slog.Error("remove chat member failed", "error", err, "subscription_id", sub.ID, "messenger_user_id", telegramID, "chat_id", chatID)
						_ = s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(ctx, sub.UserID, preferredMessengerUserID, sub.ConnectorID, domain.AuditActionSubscriptionRevokeFailed, "subscription_id="+strconv.FormatInt(sub.ID, 10), now))
					} else {
						_ = s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(ctx, sub.UserID, preferredMessengerUserID, sub.ConnectorID, domain.AuditActionSubscriptionRevokedFromChat, "subscription_id="+strconv.FormatInt(sub.ID, 10), now))
					}
				}
			}
		}

		text := s.SubscriptionExpiredText
		msg := messenger.OutgoingMessage{Text: text}
		if connectorFound {
			msg = s.BuildRenewalNotification(ctx, sub.UserID, preferredMessengerUserID, connector.StartPayload, text)
		}
		if err := s.SendUserNotification(ctx, sub.UserID, preferredMessengerUserID, msg); err != nil {
			slog.Error("send subscription expired message failed", "error", err, "subscription_id", sub.ID, "user_id", sub.UserID, "preferred_messenger_user_id", preferredMessengerUserID)
		}
		_ = s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(ctx, sub.UserID, preferredMessengerUserID, sub.ConnectorID, domain.AuditActionSubscriptionExpired, "subscription_id="+strconv.FormatInt(sub.ID, 10), now))
	}
}

func (s *Service) BuildRenewalNotification(ctx context.Context, userID int64, preferredMessengerUserID, startPayload, text string) messenger.OutgoingMessage {
	msg := messenger.OutgoingMessage{Text: text}
	payload := strings.TrimSpace(startPayload)
	if payload == "" {
		return msg
	}

	switch s.ResolvePreferredKind(ctx, userID, preferredMessengerUserID) {
	case messenger.KindMAX:
		msg.Text += fmt.Sprintf(s.RenewalCommandFormat, payload)
	default:
		if renewURL := buildBotStartURL(s.TelegramBotUsername, payload); renewURL != "" {
			msg.Buttons = [][]messenger.ActionButton{{
				{Text: s.RenewalButtonLabel, URL: renewURL},
			}}
		}
	}

	return msg
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

func shouldSendReminder(_ time.Time, _ time.Time, connector domain.Connector) bool {
	_, ok := shortTestPeriodDuration(connector)
	if !ok {
		return true
	}
	return false
}

func shouldSendExpiryNotice(connector domain.Connector) bool {
	_, ok := shortTestPeriodDuration(connector)
	return !ok
}

func shortTestPeriodDuration(connector domain.Connector) (time.Duration, bool) {
	duration, ok := connector.DurationPeriod()
	if !ok || duration <= 0 {
		return 0, false
	}
	if duration > 10*time.Minute {
		return 0, false
	}
	return duration, true
}
