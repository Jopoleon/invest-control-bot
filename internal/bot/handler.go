package bot

import (
	"github.com/Jopoleon/invest-control-bot/internal/payment"
	"github.com/Jopoleon/invest-control-bot/internal/store"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
)

// Handler orchestrates Telegram bot user flows (onboarding, consents, mock payment).
type Handler struct {
	store            store.Store
	tg               *telegram.Client
	payment          payment.Service
	recurringEnabled bool
	publicBaseURL    string
}

// NewHandler wires bot handler dependencies.
func NewHandler(st store.Store, tg *telegram.Client, paymentService payment.Service, recurringEnabled bool, publicBaseURL string) *Handler {
	return &Handler{
		store:            st,
		tg:               tg,
		payment:          paymentService,
		recurringEnabled: recurringEnabled,
		publicBaseURL:    publicBaseURL,
	}
}
