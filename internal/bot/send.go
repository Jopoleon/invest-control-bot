package bot

import (
	"context"
	"log/slog"
)

// send is a convenience wrapper for plain text outbound bot messages.
func (h *Handler) send(ctx context.Context, chatID int64, text string) {
	if err := h.tg.SendMessage(ctx, chatID, text, nil); err != nil {
		slog.Error("send message failed", "error", err, "chat_id", chatID)
	}
}
