package admin

// basePageData contains shared localization context for all admin templates.
type basePageData struct {
	Lang string
	I18N map[string]string
}

// connectorView is a template-friendly representation of connector row.
type connectorView struct {
	ID           int64
	StartPayload string
	Name         string
	ChatID       string
	ChannelURL   string
	PriceRUB     int64
	PeriodDays   int
	OfferURL     string
	PrivacyURL   string
	BotLink      string
	IsActive     bool
	ToggleTo     bool
	ToggleLabel  string
}

// connectorsPageData is context passed into connectors.html template.
type connectorsPageData struct {
	basePageData

	Notice          string
	RequiredMessage string
	Connectors      []connectorView
}

// helpPageData is context passed into help.html template.
type helpPageData struct {
	basePageData

	BotUsername string
}

// auditEventView is a template-friendly representation of audit event row.
type auditEventView struct {
	CreatedAt   string
	TelegramID  int64
	ConnectorID int64
	Connector   string
	Action      string
	Details     string
}

// eventsPageData is context passed into events.html template.
type eventsPageData struct {
	basePageData

	Notice string
	Rows   []auditEventView

	TelegramID  string
	ConnectorID string
	Action      string
	Search      string
	DateFrom    string
	DateTo      string

	SortBy   string
	SortDir  string
	PageSize int

	Page       int
	TotalPages int
	TotalItems int

	HasPrev  bool
	HasNext  bool
	FirstURL string
	PrevURL  string
	NextURL  string
	LastURL  string
}

type paymentView struct {
	ID                int64
	Provider          string
	ProviderPaymentID string
	Status            string
	TelegramID        int64
	ConnectorID       int64
	Connector         string
	AmountRUB         int64
	CreatedAt         string
	PaidAt            string
}

type subscriptionView struct {
	ID          int64
	Status      string
	TelegramID  int64
	ConnectorID int64
	Connector   string
	PaymentID   int64
	StartsAt    string
	EndsAt      string
	CreatedAt   string
}

type billingPageData struct {
	basePageData

	Notice string

	TelegramID         string
	ConnectorID        string
	PaymentStatus      string
	SubscriptionStatus string
	DateFrom           string
	DateTo             string

	Payments      []paymentView
	Subscriptions []subscriptionView
}
