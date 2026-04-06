package subscriptions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/app/periodpolicy"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
)

const (
	subscriptionRevokeRetryMaxFailures      = 3
	subscriptionRevokeRetryBackoffFirst     = 5 * time.Minute
	subscriptionRevokeRetryBackoffFollowing = 30 * time.Minute
)

var errTelegramClientNotConfigured = errors.New("telegram client is not configured")
var errMAXClientNotConfigured = errors.New("max client is not configured")

func messengerKindToDomain(kind messenger.Kind) domain.MessengerKind {
	switch kind {
	case messenger.KindMAX:
		return domain.MessengerKindMAX
	default:
		return domain.MessengerKindTelegram
	}
}

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
	RemoveTelegramChatMember    func(context.Context, string, int64) error
	RemoveMAXChatMember         func(context.Context, int64, int64) error
	SubscriptionReminderMessage func(time.Time) string
	SubscriptionExpiryMessage   func(time.Time) string
	SubscriptionExpiredText     string
	RenewalButtonLabel          string
	RenewalCommandFormat        string
	SendUserNotification        func(context.Context, int64, string, messenger.OutgoingMessage) error
	BuildTargetAuditEvent       func(context.Context, int64, string, int64, string, string, time.Time) domain.AuditEvent
	ResolvePreferredKind        func(context.Context, int64, string) messenger.Kind
	ResolveTelegramAccount      func(context.Context, int64) (domain.UserMessengerAccount, bool, error)
	ResolveMAXAccount           func(context.Context, int64) (domain.UserMessengerAccount, bool, error)
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
		if !shouldSendReminder(connector) {
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
		s.saveAuditEvent(ctx, sub.UserID, preferredMessengerUserID, sub.ConnectorID, domain.AuditActionSubscriptionReminderSent, "subscription_id="+strconv.FormatInt(sub.ID, 10), now)
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
		s.saveAuditEvent(ctx, sub.UserID, preferredMessengerUserID, sub.ConnectorID, domain.AuditActionSubscriptionExpiryNoticeSent, "subscription_id="+strconv.FormatInt(sub.ID, 10), now)
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
		if connectorFound && s.shouldDeferExpirationForPendingRebill(ctx, sub, connector, now) {
			continue
		}

		// Business status transition to expired.
		if err := s.Store.UpdateSubscriptionStatus(ctx, sub.ID, domain.SubscriptionStatusExpired, now); err != nil {
			slog.Error("update subscription status failed", "error", err, "subscription_id", sub.ID)
			continue
		}
		replacementActive := s.hasActiveReplacementSubscription(ctx, sub, now)
		sameTelegramChatAccessActive := false
		sameMAXChatAccessActive := false
		preferredMessengerUserID := ""
		preferredKind := s.ResolvePreferredKind(ctx, sub.UserID, preferredMessengerUserID)
		deliveryKind := domain.MessengerKindTelegram
		if connectorFound {
			deliveryKind = connector.DeliveryMessengerKind(messengerKindToDomain(preferredKind))
		}
		if connectorFound && deliveryKind == domain.MessengerKindTelegram {
			if chatRef := connector.ResolvedTelegramChatRef(); chatRef != "" {
				sameTelegramChatAccessActive = s.hasOtherActiveTelegramAccessForChat(ctx, sub, chatRef, now)
			}
		} else if connectorFound && deliveryKind == domain.MessengerKindMAX {
			if chatID, ok := connector.ResolvedMAXChatID(); ok {
				sameMAXChatAccessActive = s.hasOtherActiveMAXAccessForChat(ctx, sub, chatID, now)
			}
		}
		expiredAuditDetails := "subscription_id=" + strconv.FormatInt(sub.ID, 10)
		if replacementActive || sameTelegramChatAccessActive || sameMAXChatAccessActive {
			// Renewals create a new subscription row tied to the new payment. Once
			// the old period ends, the old row must transition to expired without
			// sending an "access lost" notification or revoking chat access from
			// the already-renewed user. This must also hold when the renewed access
			// came from another connector that points to the same destination chat.
			if replacementActive {
				expiredAuditDetails += ";replacement_active=true"
			}
			if sameTelegramChatAccessActive {
				expiredAuditDetails += ";same_telegram_chat_active=true"
			}
			if sameMAXChatAccessActive {
				expiredAuditDetails += ";same_max_chat_active=true"
			}
			s.saveAuditEvent(ctx, sub.UserID, "", sub.ConnectorID, domain.AuditActionSubscriptionExpired, expiredAuditDetails, now)
			continue
		}

		// Best-effort revoke from destination chat when the user's preferred
		// messenger supports membership management for that connector. Telegram
		// uses ban+immediate-unban, while MAX uses direct member removal without
		// block=true so access can be restored later.
		if connectorFound {
			switch deliveryKind {
			case domain.MessengerKindTelegram:
				if chatRef := connector.ResolvedTelegramChatRef(); chatRef != "" {
					s.attemptTelegramRevoke(ctx, sub, preferredMessengerUserID, chatRef, now, "expiry_job")
				}
			case domain.MessengerKindMAX:
				if chatID, ok := connector.ResolvedMAXChatID(); ok {
					s.attemptMAXRevoke(ctx, sub, preferredMessengerUserID, chatID, now, "expiry_job")
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
		s.saveAuditEvent(ctx, sub.UserID, preferredMessengerUserID, sub.ConnectorID, domain.AuditActionSubscriptionExpired, expiredAuditDetails, now)
	}
}

func (s *Service) ProcessFailedSubscriptionRevokes(ctx context.Context) {
	now := time.Now().UTC()
	subs, err := s.Store.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		Status: domain.SubscriptionStatusExpired,
		Limit:  s.SubscriptionJobLimit,
	})
	if err != nil {
		slog.Error("list expired subscriptions for revoke retry failed", "error", err)
		return
	}

	type auditCacheKey struct {
		userID      int64
		connectorID int64
	}
	eventsCache := make(map[auditCacheKey][]domain.AuditEvent)
	for _, sub := range subs {
		preferredKind := s.ResolvePreferredKind(ctx, sub.UserID, "")
		if s.hasActiveReplacementSubscription(ctx, sub, now) {
			continue
		}

		connector, found, err := s.Store.GetConnector(ctx, sub.ConnectorID)
		if err != nil {
			slog.Error("load connector for revoke retry failed", "error", err, "subscription_id", sub.ID, "connector_id", sub.ConnectorID)
			continue
		}
		if !found {
			continue
		}
		chatID := int64(0)
		chatOK := false
		telegramChatRef := ""
		deliveryKind := connector.DeliveryMessengerKind(messengerKindToDomain(preferredKind))
		switch deliveryKind {
		case domain.MessengerKindTelegram:
			telegramChatRef = connector.ResolvedTelegramChatRef()
			chatOK = strings.TrimSpace(telegramChatRef) != ""
			if chatOK && s.hasOtherActiveTelegramAccessForChat(ctx, sub, telegramChatRef, now) {
				continue
			}
		case domain.MessengerKindMAX:
			chatID, chatOK = connector.ResolvedMAXChatID()
			if chatOK && s.hasOtherActiveMAXAccessForChat(ctx, sub, chatID, now) {
				continue
			}
		default:
			continue
		}
		if !chatOK {
			continue
		}

		key := auditCacheKey{userID: sub.UserID, connectorID: sub.ConnectorID}
		events, cached := eventsCache[key]
		if !cached {
			var loadErr error
			events, _, loadErr = s.Store.ListAuditEvents(ctx, domain.AuditEventListQuery{
				TargetUserID: sub.UserID,
				ConnectorID:  sub.ConnectorID,
				SortBy:       "created_at",
				SortDesc:     true,
				Page:         1,
				PageSize:     200,
			})
			if loadErr != nil {
				slog.Error("list revoke retry audit events failed", "error", loadErr, "subscription_id", sub.ID, "user_id", sub.UserID, "connector_id", sub.ConnectorID)
				continue
			}
			eventsCache[key] = events
		}

		state := buildSubscriptionRevokeState(sub.ID, events)
		if state.revoked || state.manualCheckRequired || state.failureCount == 0 {
			continue
		}
		if state.failureCount >= subscriptionRevokeRetryMaxFailures {
			if s.markSubscriptionRevokeManualCheck(ctx, sub, "", now, state.failureCount, state.lastFailureReason) {
				eventsCache[key] = prependAuditEvent(eventsCache[key], s.BuildTargetAuditEvent(ctx, sub.UserID, "", sub.ConnectorID, domain.AuditActionSubscriptionRevokeManualCheck, buildSubscriptionRevokeManualCheckDetails(sub.ID, state.failureCount, state.lastFailureReason), now))
			}
			continue
		}
		if now.Sub(state.lastFailureAt) < subscriptionRevokeRetryDelay(state.failureCount) {
			continue
		}

		var (
			event     domain.AuditEvent
			attempted bool
		)
		switch deliveryKind {
		case domain.MessengerKindTelegram:
			event, attempted = s.attemptTelegramRevoke(ctx, sub, "", telegramChatRef, now, "retry_job")
		case domain.MessengerKindMAX:
			event, attempted = s.attemptMAXRevoke(ctx, sub, "", chatID, now, "retry_job")
		}
		if !attempted {
			continue
		}
		eventsCache[key] = prependAuditEvent(eventsCache[key], event)
		if event.Action == domain.AuditActionSubscriptionRevokeFailed && state.failureCount+1 >= subscriptionRevokeRetryMaxFailures {
			manualDetails := buildSubscriptionRevokeManualCheckDetails(sub.ID, state.failureCount+1, auditDetailValue(event.Details, "reason"))
			if s.markSubscriptionRevokeManualCheck(ctx, sub, "", now, state.failureCount+1, auditDetailValue(event.Details, "reason")) {
				eventsCache[key] = prependAuditEvent(eventsCache[key], s.BuildTargetAuditEvent(ctx, sub.UserID, "", sub.ConnectorID, domain.AuditActionSubscriptionRevokeManualCheck, manualDetails, now))
			}
		}
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

func shouldSendReminder(connector domain.Connector) bool {
	return !periodpolicy.Resolve(connector).SuppressPreExpiryNotifications()
}

func shouldSendExpiryNotice(connector domain.Connector) bool {
	return !periodpolicy.Resolve(connector).SuppressPreExpiryNotifications()
}

type subscriptionRevokeState struct {
	failureCount        int
	lastFailureAt       time.Time
	lastFailureReason   string
	revoked             bool
	manualCheckRequired bool
}

func buildSubscriptionRevokeState(subscriptionID int64, events []domain.AuditEvent) subscriptionRevokeState {
	state := subscriptionRevokeState{}
	for _, event := range events {
		if auditDetailInt64(event.Details, "subscription_id") != subscriptionID {
			continue
		}
		switch event.Action {
		case domain.AuditActionSubscriptionRevokedFromChat:
			state.revoked = true
			return state
		case domain.AuditActionSubscriptionRevokeManualCheck:
			state.manualCheckRequired = true
			return state
		case domain.AuditActionSubscriptionRevokeFailed:
			state.failureCount++
			if state.lastFailureAt.IsZero() || event.CreatedAt.After(state.lastFailureAt) {
				state.lastFailureAt = event.CreatedAt
				state.lastFailureReason = auditDetailValue(event.Details, "reason")
			}
		}
	}
	return state
}

func subscriptionRevokeRetryDelay(failureCount int) time.Duration {
	if failureCount <= 1 {
		return subscriptionRevokeRetryBackoffFirst
	}
	return subscriptionRevokeRetryBackoffFollowing
}

func (s *Service) attemptTelegramRevoke(ctx context.Context, sub domain.Subscription, preferredMessengerUserID, chatRef string, now time.Time, source string) (domain.AuditEvent, bool) {
	attempt := s.nextSubscriptionRevokeAttempt(ctx, sub)
	account, found, err := s.ResolveTelegramAccount(ctx, sub.UserID)
	if err != nil {
		slog.Error("resolve telegram account for revoke failed", "error", err, "subscription_id", sub.ID, "user_id", sub.UserID)
		return s.saveSubscriptionRevokeEvent(ctx, sub, preferredMessengerUserID, domain.AuditActionSubscriptionRevokeFailed, buildSubscriptionRevokeFailureDetails(sub.ID, attempt, source, "resolve_telegram_account_error"), now), true
	}
	if !found {
		slog.Warn("telegram account missing for revoke", "subscription_id", sub.ID, "user_id", sub.UserID)
		return s.saveSubscriptionRevokeEvent(ctx, sub, preferredMessengerUserID, domain.AuditActionSubscriptionRevokeFailed, buildSubscriptionRevokeFailureDetails(sub.ID, attempt, source, "telegram_account_not_found"), now), true
	}

	telegramID, parseErr := strconv.ParseInt(account.MessengerUserID, 10, 64)
	if parseErr != nil || telegramID <= 0 {
		slog.Error("invalid telegram account id for revoke", "error", parseErr, "subscription_id", sub.ID, "user_id", sub.UserID, "messenger_user_id", account.MessengerUserID)
		return s.saveSubscriptionRevokeEvent(ctx, sub, preferredMessengerUserID, domain.AuditActionSubscriptionRevokeFailed, buildSubscriptionRevokeFailureDetails(sub.ID, attempt, source, "invalid_telegram_account_id"), now), true
	}

	if err := s.removeTelegramChatMember(ctx, chatRef, telegramID); err != nil {
		slog.Error("remove chat member failed", "error", err, "subscription_id", sub.ID, "messenger_user_id", telegramID, "chat_ref", chatRef)
		reason := "remove_chat_member_failed"
		if errors.Is(err, errTelegramClientNotConfigured) {
			reason = "telegram_client_not_configured"
		}
		return s.saveSubscriptionRevokeEvent(ctx, sub, preferredMessengerUserID, domain.AuditActionSubscriptionRevokeFailed, buildSubscriptionRevokeFailureDetails(sub.ID, attempt, source, reason), now), true
	}
	return s.saveSubscriptionRevokeEvent(ctx, sub, preferredMessengerUserID, domain.AuditActionSubscriptionRevokedFromChat, buildSubscriptionRevokeSuccessDetails(sub.ID, attempt, source), now), true
}

func (s *Service) attemptMAXRevoke(ctx context.Context, sub domain.Subscription, preferredMessengerUserID string, chatID int64, now time.Time, source string) (domain.AuditEvent, bool) {
	attempt := s.nextSubscriptionRevokeAttempt(ctx, sub)
	if s.ResolveMAXAccount == nil {
		return s.saveSubscriptionRevokeEvent(ctx, sub, preferredMessengerUserID, domain.AuditActionSubscriptionRevokeFailed, buildSubscriptionRevokeFailureDetails(sub.ID, attempt, source, "max_client_not_configured"), now), true
	}
	account, found, err := s.ResolveMAXAccount(ctx, sub.UserID)
	if err != nil {
		slog.Error("resolve max account for revoke failed", "error", err, "subscription_id", sub.ID, "user_id", sub.UserID)
		return s.saveSubscriptionRevokeEvent(ctx, sub, preferredMessengerUserID, domain.AuditActionSubscriptionRevokeFailed, buildSubscriptionRevokeFailureDetails(sub.ID, attempt, source, "resolve_max_account_error"), now), true
	}
	if !found {
		slog.Warn("max account missing for revoke", "subscription_id", sub.ID, "user_id", sub.UserID)
		return s.saveSubscriptionRevokeEvent(ctx, sub, preferredMessengerUserID, domain.AuditActionSubscriptionRevokeFailed, buildSubscriptionRevokeFailureDetails(sub.ID, attempt, source, "max_account_not_found"), now), true
	}

	maxUserID, parseErr := strconv.ParseInt(account.MessengerUserID, 10, 64)
	if parseErr != nil || maxUserID <= 0 {
		slog.Error("invalid max account id for revoke", "error", parseErr, "subscription_id", sub.ID, "user_id", sub.UserID, "messenger_user_id", account.MessengerUserID)
		return s.saveSubscriptionRevokeEvent(ctx, sub, preferredMessengerUserID, domain.AuditActionSubscriptionRevokeFailed, buildSubscriptionRevokeFailureDetails(sub.ID, attempt, source, "invalid_max_account_id"), now), true
	}

	if err := s.removeMAXChatMember(ctx, chatID, maxUserID); err != nil {
		slog.Error("remove max chat member failed", "error", err, "subscription_id", sub.ID, "messenger_user_id", maxUserID, "chat_id", chatID)
		reason := "remove_max_chat_member_failed"
		if errors.Is(err, errMAXClientNotConfigured) {
			reason = "max_client_not_configured"
		}
		return s.saveSubscriptionRevokeEvent(ctx, sub, preferredMessengerUserID, domain.AuditActionSubscriptionRevokeFailed, buildSubscriptionRevokeFailureDetails(sub.ID, attempt, source, reason), now), true
	}
	return s.saveSubscriptionRevokeEvent(ctx, sub, preferredMessengerUserID, domain.AuditActionSubscriptionRevokedFromChat, buildSubscriptionRevokeSuccessDetails(sub.ID, attempt, source), now), true
}

func (s *Service) nextSubscriptionRevokeAttempt(ctx context.Context, sub domain.Subscription) int {
	events, _, err := s.Store.ListAuditEvents(ctx, domain.AuditEventListQuery{
		TargetUserID: sub.UserID,
		ConnectorID:  sub.ConnectorID,
		SortBy:       "created_at",
		SortDesc:     true,
		Page:         1,
		PageSize:     100,
	})
	if err != nil {
		slog.Error("load revoke attempts failed", "error", err, "subscription_id", sub.ID, "user_id", sub.UserID, "connector_id", sub.ConnectorID)
		return 1
	}
	attempt := 0
	for _, event := range events {
		if auditDetailInt64(event.Details, "subscription_id") != sub.ID {
			continue
		}
		if event.Action == domain.AuditActionSubscriptionRevokeFailed || event.Action == domain.AuditActionSubscriptionRevokedFromChat {
			attempt++
		}
	}
	return attempt + 1
}

func (s *Service) removeTelegramChatMember(ctx context.Context, chatRef string, userID int64) error {
	if s.RemoveTelegramChatMember != nil {
		return s.RemoveTelegramChatMember(ctx, chatRef, userID)
	}
	if s.TelegramClient == nil || !s.TelegramClient.Enabled() {
		return errTelegramClientNotConfigured
	}
	return s.TelegramClient.RemoveChatMember(ctx, chatRef, userID)
}

func (s *Service) removeMAXChatMember(ctx context.Context, chatID, userID int64) error {
	if s.RemoveMAXChatMember != nil {
		return s.RemoveMAXChatMember(ctx, chatID, userID)
	}
	return errMAXClientNotConfigured
}

func (s *Service) saveAuditEvent(ctx context.Context, userID int64, preferredMessengerUserID string, connectorID int64, action, details string, createdAt time.Time) {
	if err := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(ctx, userID, preferredMessengerUserID, connectorID, action, details, createdAt)); err != nil {
		slog.Error("save audit event failed", "error", err, "action", action, "user_id", userID, "connector_id", connectorID)
	}
}

func (s *Service) saveSubscriptionRevokeEvent(ctx context.Context, sub domain.Subscription, preferredMessengerUserID, action, details string, createdAt time.Time) domain.AuditEvent {
	event := s.BuildTargetAuditEvent(ctx, sub.UserID, preferredMessengerUserID, sub.ConnectorID, action, details, createdAt)
	if err := s.Store.SaveAuditEvent(ctx, event); err != nil {
		slog.Error("save subscription revoke audit failed", "error", err, "subscription_id", sub.ID, "action", action)
	}
	return event
}

func (s *Service) markSubscriptionRevokeManualCheck(ctx context.Context, sub domain.Subscription, preferredMessengerUserID string, now time.Time, failureCount int, reason string) bool {
	event := s.BuildTargetAuditEvent(ctx, sub.UserID, preferredMessengerUserID, sub.ConnectorID, domain.AuditActionSubscriptionRevokeManualCheck, buildSubscriptionRevokeManualCheckDetails(sub.ID, failureCount, reason), now)
	if err := s.Store.SaveAuditEvent(ctx, event); err != nil {
		slog.Error("save revoke manual-check audit failed", "error", err, "subscription_id", sub.ID)
		return false
	}
	slog.Warn("subscription revoke requires manual check", "subscription_id", sub.ID, "user_id", sub.UserID, "connector_id", sub.ConnectorID, "failed_attempts", failureCount, "reason", reason)
	return true
}

func buildSubscriptionRevokeFailureDetails(subscriptionID int64, attempt int, source, reason string) string {
	return "subscription_id=" + strconv.FormatInt(subscriptionID, 10) +
		";attempt=" + strconv.Itoa(attempt) +
		";source=" + strings.TrimSpace(source) +
		";reason=" + strings.TrimSpace(reason)
}

func buildSubscriptionRevokeSuccessDetails(subscriptionID int64, attempt int, source string) string {
	return "subscription_id=" + strconv.FormatInt(subscriptionID, 10) +
		";attempt=" + strconv.Itoa(attempt) +
		";source=" + strings.TrimSpace(source)
}

func buildSubscriptionRevokeManualCheckDetails(subscriptionID int64, failureCount int, reason string) string {
	details := "subscription_id=" + strconv.FormatInt(subscriptionID, 10) +
		";failed_attempts=" + strconv.Itoa(failureCount)
	if strings.TrimSpace(reason) != "" {
		details += ";reason=" + strings.TrimSpace(reason)
	}
	return details
}

func auditDetailInt64(details, key string) int64 {
	value, _ := strconv.ParseInt(auditDetailValue(details, key), 10, 64)
	return value
}

func auditDetailValue(details, key string) string {
	prefix := strings.TrimSpace(key) + "="
	for _, part := range strings.Split(details, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(part, prefix))
		}
	}
	return ""
}

func prependAuditEvent(events []domain.AuditEvent, event domain.AuditEvent) []domain.AuditEvent {
	return append([]domain.AuditEvent{event}, events...)
}

func (s *Service) shouldDeferExpirationForPendingRebill(ctx context.Context, sub domain.Subscription, connector domain.Connector, now time.Time) bool {
	timing := periodpolicy.Resolve(connector)
	if !timing.ShouldDeferExpiration(now, sub.EndsAt) {
		return false
	}
	pending, found, err := s.Store.GetPendingRebillPaymentBySubscription(ctx, sub.ID)
	if err != nil {
		slog.Error("load pending rebill before expiration failed", "error", err, "subscription_id", sub.ID)
		return false
	}
	return found && pending.Status == domain.PaymentStatusPending
}

func (s *Service) hasActiveReplacementSubscription(ctx context.Context, sub domain.Subscription, now time.Time) bool {
	latestSub, found, err := s.Store.GetLatestSubscriptionByUserConnector(ctx, sub.UserID, sub.ConnectorID)
	if err != nil {
		slog.Error("load latest subscription during expiration failed", "error", err, "subscription_id", sub.ID, "user_id", sub.UserID, "connector_id", sub.ConnectorID)
		return false
	}
	if !found || latestSub.ID == sub.ID {
		return false
	}
	return latestSub.IsCurrentActiveAt(now)
}

// hasOtherActiveTelegramAccessForChat reports whether the user already has
// another active subscription that grants access to the same Telegram chat.
// This prevents false "access lost" handling when the user renewed access via
// a different connector that points to the same destination.
func (s *Service) hasOtherActiveTelegramAccessForChat(ctx context.Context, sub domain.Subscription, chatRef string, now time.Time) bool {
	subs, err := s.Store.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		UserID: sub.UserID,
		Status: domain.SubscriptionStatusActive,
		Limit:  200,
	})
	if err != nil {
		slog.Error("list active subscriptions for telegram chat replacement failed", "error", err, "subscription_id", sub.ID, "user_id", sub.UserID, "chat_ref", chatRef)
		return false
	}
	for _, candidate := range subs {
		if candidate.ID == sub.ID || !candidate.IsCurrentActiveAt(now) {
			continue
		}
		connector, found, err := s.Store.GetConnector(ctx, candidate.ConnectorID)
		if err != nil {
			slog.Error("load connector for telegram chat replacement failed", "error", err, "subscription_id", candidate.ID, "connector_id", candidate.ConnectorID, "user_id", sub.UserID)
			continue
		}
		if !found {
			continue
		}
		candidateChatRef := connector.ResolvedTelegramChatRef()
		if candidateChatRef != "" && candidateChatRef == chatRef {
			return true
		}
	}
	return false
}

func (s *Service) hasOtherActiveMAXAccessForChat(ctx context.Context, sub domain.Subscription, chatID int64, now time.Time) bool {
	subs, err := s.Store.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		UserID: sub.UserID,
		Status: domain.SubscriptionStatusActive,
		Limit:  200,
	})
	if err != nil {
		slog.Error("list active subscriptions for max chat replacement failed", "error", err, "subscription_id", sub.ID, "user_id", sub.UserID, "chat_id", chatID)
		return false
	}
	for _, candidate := range subs {
		if candidate.ID == sub.ID || !candidate.IsCurrentActiveAt(now) {
			continue
		}
		connector, found, err := s.Store.GetConnector(ctx, candidate.ConnectorID)
		if err != nil {
			slog.Error("load connector for max chat replacement failed", "error", err, "subscription_id", candidate.ID, "connector_id", candidate.ConnectorID, "user_id", sub.UserID)
			continue
		}
		if !found {
			continue
		}
		candidateChatID, ok := connector.ResolvedMAXChatID()
		if ok && candidateChatID == chatID {
			return true
		}
	}
	return false
}
