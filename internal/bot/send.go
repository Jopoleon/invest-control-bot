package bot

import (
	"context"
	"log"
)

// send is a convenience wrapper for plain text outbound bot messages.
func (h *Handler) send(ctx context.Context, chatID int64, text string) {
	if err := h.tg.SendMessage(ctx, chatID, text, nil); err != nil {
		log.Printf("send message failed: %v", err)
	}
}
