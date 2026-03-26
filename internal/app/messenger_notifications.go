package app

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// sendUserNotification routes user-facing app notifications through the linked
// messenger account that matches the external id stored in legacy payment and
// subscription records. While the schema is still in mixed mode, this keeps
// Telegram flows working and allows MAX-originated payments to notify MAX users.
func (a *application) sendUserNotification(ctx context.Context, userID, legacyExternalID int64, msg messenger.OutgoingMessage) error {
	if userID > 0 {
		accounts, err := a.store.ListUserMessengerAccounts(ctx, userID)
		if err != nil {
			return fmt.Errorf("list user messenger accounts: %w", err)
		}
		targetExternalID := strconv.FormatInt(legacyExternalID, 10)
		for _, account := range accounts {
			if account.ExternalUserID != targetExternalID {
				continue
			}
			if err := a.sendViaMessengerAccount(ctx, account, msg); err == nil {
				return nil
			} else {
				slog.Warn("send via matched messenger account failed", "user_id", userID, "messenger_kind", account.MessengerKind, "external_user_id", account.ExternalUserID, "error", err)
			}
		}
	}

	if legacyExternalID <= 0 {
		return nil
	}

	telegramErr := a.telegramClient.Send(ctx, messenger.UserRef{
		Kind:   messenger.KindTelegram,
		ChatID: legacyExternalID,
	}, msg)
	if telegramErr == nil {
		return nil
	}

	if a.maxSender == nil {
		return telegramErr
	}
	maxErr := a.maxSender.Send(ctx, messenger.UserRef{
		Kind:   messenger.KindMAX,
		ChatID: legacyExternalID,
	}, msg)
	if maxErr == nil {
		slog.Warn("notification delivery fell back from telegram to MAX", "user_id", userID, "legacy_external_id", legacyExternalID)
		return nil
	}
	return fmt.Errorf("telegram send failed: %v; max send failed: %w", telegramErr, maxErr)
}

func (a *application) sendViaMessengerAccount(ctx context.Context, account domain.UserMessengerAccount, msg messenger.OutgoingMessage) error {
	externalID, err := strconv.ParseInt(account.ExternalUserID, 10, 64)
	if err != nil || externalID <= 0 {
		return fmt.Errorf("invalid external user id %q for %s", account.ExternalUserID, account.MessengerKind)
	}

	switch account.MessengerKind {
	case domain.MessengerKindMAX:
		if a.maxSender == nil {
			return fmt.Errorf("max sender is not configured")
		}
		return a.maxSender.Send(ctx, messenger.UserRef{Kind: messenger.KindMAX, ChatID: externalID}, msg)
	case domain.MessengerKindTelegram:
		return a.telegramClient.Send(ctx, messenger.UserRef{Kind: messenger.KindTelegram, ChatID: externalID}, msg)
	default:
		return fmt.Errorf("unsupported messenger kind %q", account.MessengerKind)
	}
}
