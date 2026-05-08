package bot

import (
	"context"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/payment"
	"github.com/Jopoleon/invest-control-bot/internal/store"
)

// TelegramAccessLinkBuilder creates a fresh Telegram invite link for an
// existing subscription. The app layer provides the implementation so this
// package can keep Telegram API details out of the messenger-neutral bot flow.
type TelegramAccessLinkBuilder func(context.Context, int64, domain.Connector, domain.Subscription) (string, error)

// Handler orchestrates messenger-agnostic user flows while transport-specific
// delivery/parsing is pushed to adapters and the sender contract.
type Handler struct {
	store                     store.Store
	sender                    messenger.Sender
	payment                   payment.Service
	recurringEnabled          bool
	publicBaseURL             string
	encryptionKey             string
	telegramAccessLinkBuilder TelegramAccessLinkBuilder
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

// SetTelegramAccessLinkBuilder enables Telegram-native access refreshes for
// user-facing subscription menus without coupling bot handlers to Telegram API.
func (h *Handler) SetTelegramAccessLinkBuilder(builder TelegramAccessLinkBuilder) {
	h.telegramAccessLinkBuilder = builder
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
