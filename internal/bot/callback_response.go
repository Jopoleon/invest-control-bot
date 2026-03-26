package bot

import (
	"context"

	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// respondToAction updates the existing interactive message when the transport
// gives us a stable numeric message id. When the transport cannot provide one
// yet (current MAX callback payloads expose string mids), we fall back to
// sending a fresh message so the interaction stays functional.
func (h *Handler) respondToAction(ctx context.Context, cb messenger.IncomingAction, msg messenger.OutgoingMessage) error {
	if cb.ChatID == 0 {
		return nil
	}
	if cb.MessageID > 0 {
		return h.sender.Edit(ctx, messenger.MessageRef{
			Kind:      cb.Ref.Kind,
			ChatID:    cb.ChatID,
			MessageID: cb.MessageID,
		}, msg)
	}
	return h.sender.Send(ctx, messenger.UserRef{
		Kind:   cb.User.Kind,
		ChatID: cb.ChatID,
	}, msg)
}
