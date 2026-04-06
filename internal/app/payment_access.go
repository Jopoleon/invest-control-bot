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

	// Single-use links are safer than exposing the chat/channel URL after payment.
	inviteName := fmt.Sprintf("paid-u%d-c%d", userID, connector.ID)
	expireAt := time.Now().UTC().Add(24 * time.Hour)
	link, err := a.telegramClient.CreateSingleUseInviteLink(ctx, chatRef, inviteName, expireAt)
	if err != nil {
		return "", fmt.Errorf("create single-use invite link for chat_ref=%s: %w", chatRef, err)
	}
	return link, nil
}
