package payment

import "context"

// Request contains minimum data needed to create checkout URL.
type Request struct {
	UserTelegramID int64
	ConnectorID    int64
	AmountRUB      int64
	InvoiceID      string
	Description    string
}

// Service abstracts payment provider integration behind uniform API.
type Service interface {
	CreateCheckoutURL(ctx context.Context, req Request) (string, error)
	ProviderName() string
}
