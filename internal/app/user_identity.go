package app

import (
	"context"
	"strconv"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

// resolveTelegramUser loads an existing user through the messenger-identity layer.
// Public recurring pages still receive a legacy Telegram-based token, so they need
// a read-only bridge until subscriptions and payments move to internal user IDs.
func (a *application) resolveTelegramUser(ctx context.Context, telegramID int64) (domain.User, bool, error) {
	if telegramID <= 0 {
		return domain.User{}, false, nil
	}
	return a.store.GetUserByMessenger(ctx, domain.MessengerKindTelegram, strconv.FormatInt(telegramID, 10))
}

func (a *application) resolveUserByMessengerUserID(ctx context.Context, messengerUserID int64) (domain.User, bool, error) {
	if messengerUserID <= 0 {
		return domain.User{}, false, nil
	}
	raw := strconv.FormatInt(messengerUserID, 10)
	if user, found, err := a.store.GetUserByMessenger(ctx, domain.MessengerKindTelegram, raw); err != nil || found {
		return user, found, err
	}
	return a.store.GetUserByMessenger(ctx, domain.MessengerKindMAX, raw)
}
