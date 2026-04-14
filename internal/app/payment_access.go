package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

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
	if a.telegramClient == nil {
		return "", nil
	}
	if a.resolvePreferredMessengerKind(ctx, userID, "") != messenger.KindTelegram {
		return "", nil
	}

	chatRef := connector.ResolvedTelegramChatRef()
	if strings.TrimSpace(chatRef) == "" {
		return "", nil
	}

	// Single-use links are safer than exposing the chat/channel URL after
	// payment. Their TTL is capped by the paid period when we already know the
	// subscription row, so the button in chat history cannot stay valid much
	// longer than the access itself.
	inviteName := fmt.Sprintf("paid-u%d-c%d", userID, connector.ID)
	expireAt := time.Now().UTC().Add(24 * time.Hour)
	if sub.ID > 0 && !sub.EndsAt.IsZero() && sub.EndsAt.After(time.Now().UTC()) {
		expireAt = sub.EndsAt.UTC()
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
