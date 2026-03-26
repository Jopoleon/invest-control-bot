package bot

import (
	"context"

	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/payment"
	"github.com/Jopoleon/invest-control-bot/internal/store"
)

// Handler orchestrates messenger-agnostic user flows while transport-specific
// delivery/parsing is pushed to adapters and the sender contract.
type Handler struct {
	store            store.Store
	sender           messenger.Sender
	payment          payment.Service
	recurringEnabled bool
	publicBaseURL    string
	encryptionKey    string
}

// NewHandler wires use-case dependencies independently from a concrete messenger.
func NewHandler(st store.Store, sender messenger.Sender, paymentService payment.Service, recurringEnabled bool, publicBaseURL, encryptionKey string) *Handler {
	return &Handler{
		store:            st,
		sender:           sender,
		payment:          paymentService,
		recurringEnabled: recurringEnabled,
		publicBaseURL:    publicBaseURL,
		encryptionKey:    encryptionKey,
	}
}

// HandleIncomingMessage is the messenger-neutral entrypoint used by transport
// adapters after they map raw update payloads into internal event objects.
func (h *Handler) HandleIncomingMessage(ctx context.Context, msg messenger.IncomingMessage) {
	h.handleMessage(ctx, msg)
}

// HandleIncomingAction is the messenger-neutral callback entrypoint used by
// transport adapters for button interactions.
func (h *Handler) HandleIncomingAction(ctx context.Context, action messenger.IncomingAction) {
	h.handleCallback(ctx, action)
}
