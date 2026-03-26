package bot

import (
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
