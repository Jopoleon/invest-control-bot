package bot

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

// resolveTelegramUser ensures there is one internal user linked to the current
// Telegram identity and returns that user record for further bot logic.
func (h *Handler) resolveTelegramUser(ctx context.Context, telegramID int64, username string) (domain.User, bool) {
	if telegramID <= 0 {
		return domain.User{}, false
	}

	user, _, err := h.store.GetOrCreateUserByMessenger(
		ctx,
		domain.MessengerKindTelegram,
		strconv.FormatInt(telegramID, 10),
		username,
	)
	if err != nil {
		slog.Error("resolve telegram user failed", "error", err, "telegram_id", telegramID)
		return domain.User{}, false
	}
	return user, true
}
