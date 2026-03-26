package bot

import (
	"context"

	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/go-telegram/bot/models"
)

// HandleUpdate is the Telegram transport boundary: it maps Telegram DTOs to
// internal messenger-neutral events and keeps the rest of the bot package free
// from direct Telegram API types.
func (h *Handler) HandleUpdate(ctx context.Context, update *models.Update) {
	if update == nil {
		return
	}
	if update.CallbackQuery != nil {
		cb := update.CallbackQuery
		if cb.Message.Message == nil {
			return
		}
		h.handleCallback(ctx, messenger.IncomingAction{
			Ref:       actionRef(cb.ID),
			User:      userIdentity(cb.From.ID, cb.From.Username),
			ChatID:    cb.Message.Message.Chat.ID,
			MessageID: cb.Message.Message.ID,
			Data:      cb.Data,
		})
		return
	}
	if update.Message != nil {
		msg := update.Message
		h.handleMessage(ctx, messenger.IncomingMessage{
			User:   userIdentity(msg.From.ID, msg.From.Username),
			ChatID: msg.Chat.ID,
			Text:   msg.Text,
		})
	}
}
