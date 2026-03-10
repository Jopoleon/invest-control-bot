package bot

import (
	"context"

	"github.com/go-telegram/bot/models"
)

// HandleUpdate routes Telegram updates to message/callback handlers.
func (h *Handler) HandleUpdate(ctx context.Context, update *models.Update) {
	if update == nil {
		return
	}
	if update.CallbackQuery != nil {
		h.handleCallback(ctx, update.CallbackQuery)
		return
	}
	if update.Message != nil {
		h.handleMessage(ctx, update.Message)
	}
}
