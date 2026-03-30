package bot

import (
	"context"
	"log/slog"

	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// send is a convenience wrapper for plain text outbound bot messages.
func (h *Handler) send(ctx context.Context, chatID int64, text string) {
	if err := h.sender.Send(ctx, chatRef(chatID), messenger.OutgoingMessage{Text: text}); err != nil {
		slog.Error("send message failed", "error", err, "chat_id", chatID)
	}
}

func (h *Handler) sendTo(ctx context.Context, chatID int64, user messenger.UserIdentity, text string) {
	if err := h.sender.Send(ctx, recipientRef(chatID, user), messenger.OutgoingMessage{Text: text}); err != nil {
		slog.Error("send message failed", "error", err, "chat_id", chatID, "messenger_kind", user.Kind, "messenger_user_id", user.ID)
	}
}
