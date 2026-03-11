package admin

// basePageData contains shared localization context for all admin templates.
type basePageData struct {
	Lang string
	I18N map[string]string
}

// connectorView is a template-friendly representation of connector row.
type connectorView struct {
	ID           string
	StartPayload string
	Name         string
	ChatID       string
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
	ConnectorID string
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
