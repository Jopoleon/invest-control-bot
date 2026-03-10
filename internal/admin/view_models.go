package admin

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
	Notice          string
	RequiredMessage string
	Connectors      []connectorView
}

// helpPageData is context passed into help.html template.
type helpPageData struct {
	BotUsername string
}
