package payment

import "context"

// Request contains minimum data needed to create checkout URL.
type Request struct {
	ConnectorID int64
	AmountRUB   int64
	// InvoiceID is the merchant-side payment identifier passed to provider
	// checkout. For Robokassa this is the canonical `InvoiceID` / `InvId` and is
	// persisted in payments.token.
	InvoiceID       string
	Description     string
	EnableRecurring bool
}

// Service abstracts payment provider integration behind uniform API.
type Service interface {
	CreateCheckoutURL(ctx context.Context, req Request) (string, error)
	ProviderName() string
	IsTestMode() bool
}

// RebillRequest contains data for provider-side recurring charge creation.
type RebillRequest struct {
	// InvoiceID is the new merchant-side recurring payment identifier. For
	// Robokassa it is sent as `InvoiceID` and persisted in payments.token.
	InvoiceID string
	// PreviousInvoiceID is the parent successful merchant invoice reference. For
	// Robokassa it maps to the previous payment `InvoiceID` / `InvId`.
	PreviousInvoiceID string
	AmountRUB         int64
	Description       string
}

// RebillProvider is implemented by providers that support server-side recurring charges.
type RebillProvider interface {
	CreateRebill(ctx context.Context, req RebillRequest) error
}
