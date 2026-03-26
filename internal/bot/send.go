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
