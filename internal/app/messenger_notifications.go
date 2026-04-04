package app

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// sendUserNotification routes app-level notifications through the linked
// messenger account of the internal user. When the caller still has a preferred
// messenger user id from mixed-mode records, it can be passed to keep MAX flows
// targeting MAX and Telegram flows targeting Telegram during the transition.
func (a *application) sendUserNotification(ctx context.Context, userID int64, preferredMessengerUserID string, msg messenger.OutgoingMessage) error {
	accounts, err := a.loadUserMessengerAccounts(ctx, userID)
	if err != nil {
		return err
	}
	if len(accounts) > 0 {
		preferred, found := pickPreferredMessengerAccount(accounts, preferredMessengerUserID)
		if found {
			if err := a.sendViaMessengerAccount(ctx, preferred, msg); err == nil {
				return nil
			} else {
				slog.Warn("send via preferred messenger account failed", "user_id", userID, "messenger_kind", preferred.MessengerKind, "messenger_user_id", preferred.MessengerUserID, "error", err)
			}
		}
		for _, account := range accounts {
			if found && account.MessengerKind == preferred.MessengerKind && account.MessengerUserID == preferred.MessengerUserID {
				continue
			}
			if err := a.sendViaMessengerAccount(ctx, account, msg); err == nil {
				return nil
			}
		}
	}

	if preferredMessengerUserID == "" {
		return nil
	}
	chatID, err := strconv.ParseInt(preferredMessengerUserID, 10, 64)
	if err != nil || chatID <= 0 {
		return nil
	}

	var telegramErr error
	if a.telegramClient == nil {
		telegramErr = fmt.Errorf("telegram sender is not configured")
	} else {
		telegramErr = a.telegramClient.Send(ctx, messenger.UserRef{
			Kind:   messenger.KindTelegram,
			ChatID: chatID,
		}, msg)
	}
	if telegramErr == nil {
		return nil
	}

	if a.maxSender == nil {
		return telegramErr
	}
	maxErr := a.maxSender.Send(ctx, messenger.UserRef{
		Kind:   messenger.KindMAX,
		ChatID: chatID,
	}, msg)
	if maxErr == nil {
		slog.Warn("notification delivery fell back from telegram to MAX", "user_id", userID, "preferred_messenger_user_id", preferredMessengerUserID)
		return nil
	}
	return fmt.Errorf("telegram send failed: %v; max send failed: %w", telegramErr, maxErr)
}

func (a *application) resolvePreferredMessengerKind(ctx context.Context, userID int64, preferredMessengerUserID string) messenger.Kind {
	account, found, err := a.resolvePreferredMessengerAccount(ctx, userID, preferredMessengerUserID)
	if err != nil || !found {
		return messenger.KindTelegram
	}
	switch account.MessengerKind {
	case domain.MessengerKindMAX:
		return messenger.KindMAX
	default:
		return messenger.KindTelegram
	}
}

func (a *application) resolvePreferredMessengerAccount(ctx context.Context, userID int64, preferredMessengerUserID string) (domain.UserMessengerAccount, bool, error) {
	accounts, err := a.loadUserMessengerAccounts(ctx, userID)
	if err != nil {
		return domain.UserMessengerAccount{}, false, err
	}
	account, found := pickPreferredMessengerAccount(accounts, preferredMessengerUserID)
	return account, found, nil
}

func (a *application) resolveTelegramMessengerAccount(ctx context.Context, userID int64) (domain.UserMessengerAccount, bool, error) {
	accounts, err := a.loadUserMessengerAccounts(ctx, userID)
	if err != nil {
		return domain.UserMessengerAccount{}, false, err
	}
	for _, account := range accounts {
		if account.MessengerKind == domain.MessengerKindTelegram {
			return account, true, nil
		}
	}
	return domain.UserMessengerAccount{}, false, nil
}

func (a *application) resolveTelegramMessengerUserID(ctx context.Context, userID int64) (int64, bool, error) {
	account, found, err := a.resolveTelegramMessengerAccount(ctx, userID)
	if err != nil || !found {
		return 0, false, err
	}
	telegramID, parseErr := strconv.ParseInt(strings.TrimSpace(account.MessengerUserID), 10, 64)
	if parseErr != nil || telegramID <= 0 {
		return 0, false, nil
	}
	return telegramID, true, nil
}

func (a *application) resolveMAXMessengerAccount(ctx context.Context, userID int64) (domain.UserMessengerAccount, bool, error) {
	accounts, err := a.loadUserMessengerAccounts(ctx, userID)
	if err != nil {
		return domain.UserMessengerAccount{}, false, err
	}
	for _, account := range accounts {
		if account.MessengerKind == domain.MessengerKindMAX {
			return account, true, nil
		}
	}
	return domain.UserMessengerAccount{}, false, nil
}

func (a *application) resolveMAXMessengerUserID(ctx context.Context, userID int64) (int64, bool, error) {
	account, found, err := a.resolveMAXMessengerAccount(ctx, userID)
	if err != nil || !found {
		return 0, false, err
	}
	maxID, parseErr := strconv.ParseInt(strings.TrimSpace(account.MessengerUserID), 10, 64)
	if parseErr != nil || maxID <= 0 {
		return 0, false, nil
	}
	return maxID, true, nil
}

func (a *application) loadUserMessengerAccounts(ctx context.Context, userID int64) ([]domain.UserMessengerAccount, error) {
	if userID <= 0 {
		return nil, nil
	}
	accounts, err := a.store.ListUserMessengerAccounts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list user messenger accounts: %w", err)
	}
	return accounts, nil
}

func pickPreferredMessengerAccount(accounts []domain.UserMessengerAccount, preferredMessengerUserID string) (domain.UserMessengerAccount, bool) {
	preferredMessengerUserID = strings.TrimSpace(preferredMessengerUserID)
	if preferredMessengerUserID != "" {
		for _, account := range accounts {
			if account.MessengerUserID == preferredMessengerUserID {
				return account, true
			}
		}
	}
	for _, account := range accounts {
		if account.MessengerKind == domain.MessengerKindTelegram {
			return account, true
		}
	}
	for _, account := range accounts {
		if account.MessengerKind == domain.MessengerKindMAX {
			return account, true
		}
	}
	if len(accounts) == 0 {
		return domain.UserMessengerAccount{}, false
	}
	return accounts[0], true
}

func formatPreferredMessengerUserID(raw int64) string {
	if raw <= 0 {
		return ""
	}
	return strconv.FormatInt(raw, 10)
}

func (a *application) sendViaMessengerAccount(ctx context.Context, account domain.UserMessengerAccount, msg messenger.OutgoingMessage) error {
	externalID, err := strconv.ParseInt(account.MessengerUserID, 10, 64)
	if err != nil || externalID <= 0 {
		return fmt.Errorf("invalid messenger user id %q for %s", account.MessengerUserID, account.MessengerKind)
	}

	switch account.MessengerKind {
	case domain.MessengerKindMAX:
		if a.maxSender == nil {
			return fmt.Errorf("max sender is not configured")
		}
		return a.maxSender.Send(ctx, messenger.UserRef{Kind: messenger.KindMAX, UserID: externalID}, msg)
	case domain.MessengerKindTelegram:
		if a.telegramClient == nil {
			return fmt.Errorf("telegram sender is not configured")
		}
		return a.telegramClient.Send(ctx, messenger.UserRef{Kind: messenger.KindTelegram, ChatID: externalID}, msg)
	default:
		return fmt.Errorf("unsupported messenger kind %q", account.MessengerKind)
	}
}
