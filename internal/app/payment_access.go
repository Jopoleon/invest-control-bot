package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

const telegramPaymentInviteLinkTTL = 36 * time.Hour

func (a *application) buildTelegramPaymentAccessLink(ctx context.Context, userID int64, connector domain.Connector) (string, error) {
	return a.buildTelegramPaymentAccessLinkForSubscription(ctx, userID, connector, domain.Subscription{})
}

// buildTelegramPaymentAccessLinkForSubscription creates a Telegram-native
// invite link for a paid subscription and persists that exact link when the
// subscription row is already known.
//
// Persistence matters because later expiry/revoke flows need the concrete
// invite-link string to revoke it via Telegram Bot API. The link itself lives
// on Telegram's side, not on our domain.
//
// TODO: Add a small cleanup/reconciliation job for stale invite-link rows that
// are already expired and revoked in Telegram but still not marked in DB due to
// transient API/database failures.
func (a *application) buildTelegramPaymentAccessLinkForSubscription(ctx context.Context, userID int64, connector domain.Connector, sub domain.Subscription) (string, error) {
	if a.resolvePreferredMessengerKind(ctx, userID, "") != messenger.KindTelegram {
		return "", nil
	}
	return a.buildTelegramSubscriptionAccessLink(ctx, userID, connector, sub)
}

func (a *application) buildTelegramSubscriptionAccessLink(ctx context.Context, userID int64, connector domain.Connector, sub domain.Subscription) (string, error) {
	if a.telegramClient == nil {
		return "", nil
	}
	chatRef := connector.ResolvedTelegramChatRef()
	if strings.TrimSpace(chatRef) == "" {
		return "", nil
	}

	// Single-use links are safer than exposing the chat/channel URL after
	// payment. They are valid long enough for normal onboarding friction but
	// still short-lived, and they are additionally capped by the paid period
	// when we already know the subscription row.
	inviteName := fmt.Sprintf("paid-u%d-c%d", userID, connector.ID)
	now := time.Now().UTC()
	expireAt := now.Add(telegramPaymentInviteLinkTTL)
	if sub.ID > 0 && !sub.EndsAt.IsZero() && sub.EndsAt.After(now) {
		subEndsAt := sub.EndsAt.UTC()
		if subEndsAt.Before(expireAt) {
			expireAt = subEndsAt
		}
	}
	link, err := a.telegramClient.CreateSingleUseInviteLink(ctx, chatRef, inviteName, expireAt)
	if err != nil {
		return "", fmt.Errorf("create single-use invite link for chat_ref=%s: %w", chatRef, err)
	}
	if strings.TrimSpace(link) == "" || sub.ID <= 0 {
		return link, nil
	}
	expiresAt := expireAt.UTC()
	if err := a.store.SaveTelegramInviteLink(ctx, domain.TelegramInviteLink{
		UserID:         userID,
		ConnectorID:    connector.ID,
		SubscriptionID: sub.ID,
		ChatRef:        chatRef,
		InviteLink:     strings.TrimSpace(link),
		ExpiresAt:      &expiresAt,
		CreatedAt:      time.Now().UTC(),
	}); err != nil {
		return "", fmt.Errorf("persist telegram invite link for subscription_id=%d: %w", sub.ID, err)
	}
	return link, nil
}
