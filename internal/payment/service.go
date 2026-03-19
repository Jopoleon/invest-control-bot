package payment

import "context"

// Request contains minimum data needed to create checkout URL.
type Request struct {
	UserTelegramID  int64
	ConnectorID     int64
	AmountRUB       int64
	InvoiceID       string
	Description     string
	EnableRecurring bool
}

// Service abstracts payment provider integration behind uniform API.
type Service interface {
	CreateCheckoutURL(ctx context.Context, req Request) (string, error)
	ProviderName() string
}

// RebillRequest contains data for provider-side recurring charge creation.
type RebillRequest struct {
	InvoiceID         string
	PreviousInvoiceID string
	AmountRUB         int64
	Description       string
}

// RebillProvider is implemented by providers that support server-side recurring charges.
type RebillProvider interface {
	CreateRebill(ctx context.Context, req RebillRequest) error
}
