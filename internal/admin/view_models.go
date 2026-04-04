package admin

// basePageData contains shared localization context for all admin templates.
type basePageData struct {
	Lang       string
	I18N       map[string]string
	CSRFToken  string
	TopbarPath string
	ActiveNav  string
}

// connectorView is a template-friendly representation of connector row.
type connectorView struct {
	ID              int64
	StartPayload    string
	Name            string
	ChatID          string
	TelegramURL     string
	MAXChannelURL   string
	PriceRUB        int64
	PeriodLabel     string
	OfferURL        string
	PrivacyURL      string
	TelegramBotLink string
	MAXBotLink      string
	MAXStartCommand string
	IsActive        bool
	ActiveLabel     string
	ActiveClass     string
	ToggleTo        bool
	ToggleLabel     string
}

// connectorsPageData is context passed into connectors.html template.
type connectorsPageData struct {
	basePageData

	Notice              string
	RequiredMessage     string
	ExportURL           string
	TelegramBotUsername string
	MAXBotUsername      string
	Connectors          []connectorView
}

// helpPageData is context passed into help.html template.
type helpPageData struct {
	basePageData

	BotUsername string
}

type loginPageData struct {
	basePageData

	Notice string
	Next   string
}

// auditEventView is a template-friendly representation of audit event row.
type auditEventView struct {
	CreatedAt             string
	ActorType             string
	TargetAccount         string
	TargetMessengerKind   string
	TargetMessengerUserID string
	ConnectorID           int64
	Connector             string
	Action                string
	Details               string
}

// eventsPageData is context passed into events.html template.
type eventsPageData struct {
	basePageData

	Notice    string
	ExportURL string
	Rows      []auditEventView

	ActorType       string
	MessengerKind   string
	MessengerUserID string
	ConnectorID     string
	Action          string
	Search          string
	DateFrom        string
	DateTo          string

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
	UserID            int64
	PrimaryAccount    string
	Provider          string
	ProviderPaymentID string
	Status            string
	StatusLabel       string
	StatusClass       string
	AutoPayEnabled    bool
	AutoPayLabel      string
	AutoPayClass      string
	ConnectorID       int64
	Connector         string
	AmountRUB         int64
	AccessLabel       string
	AccessClass       string
	CreatedAt         string
	PaidAt            string
}

type subscriptionView struct {
	ID               int64
	UserID           int64
	PrimaryAccount   string
	Status           string
	StatusLabel      string
	StatusClass      string
	AutoPayEnabled   bool
	AutoPayLabel     string
	AutoPayClass     string
	ConnectorID      int64
	Connector        string
	PaymentID        int64
	AccessLabel      string
	AccessClass      string
	StartsAt         string
	EndsAt           string
	CreatedAt        string
	CanRevoke        bool
	RevokeURL        string
	CanSendPayLink   bool
	PaymentLinkURL   string
	CanTriggerRebill bool
	RebillURL        string
}

type billingPageData struct {
	basePageData

	Notice string

	UserID             string
	TelegramID         string
	ConnectorID        string
	PaymentStatus      string
	SubscriptionStatus string
	DateFrom           string
	DateTo             string

	Payments               []paymentView
	Subscriptions          []subscriptionView
	PaymentsExportURL      string
	SubscriptionsExportURL string
	Summary                billingSummaryView
	Groups                 []billingGroupView
}

type billingSummaryView struct {
	TotalPayments       int
	PaidPayments        int
	FailedPayments      int
	ActiveSubscriptions int
	PaidAmountRUB       int64
	PendingAmountRUB    int64
}

type billingGroupView struct {
	ConnectorID         int64
	Connector           string
	PaidAmountRUB       int64
	PaidPayments        int
	PendingPayments     int
	FailedPayments      int
	ActiveSubscriptions int
}

type messengerAccountView struct {
	KindLabel       string
	MessengerUserID string
	Username        string
	Display         string
}

type userView struct {
	UserID              int64
	DisplayName         string
	CanDirectMessage    bool
	DirectMessageTarget string
	DirectMessageKind   string
	CanOpenDirectChat   bool
	DirectChatURL       string
	PrimaryAccount      string
	LinkedAccounts      []messengerAccountView
	HasTelegramIdentity bool
	FullName            string
	Phone               string
	Email               string
	AutoPay             string
	AutoPayClass        string
	UpdatedAt           string
	DetailURL           string
}

type usersPageData struct {
	basePageData

	Notice     string
	UserID     string
	TelegramID string
	Search     string
	ExportURL  string
	Users      []userView
}

type userDetailPageData struct {
	basePageData

	Notice            string
	BackURL           string
	MessageActionURL  string
	AutopayCancelURL  string
	User              userView
	RecurringSummary  recurringSummaryView
	Consents          []consentView
	RecurringConsents []recurringConsentView
	Payments          []paymentView
	Subscriptions     []subscriptionView
	Events            []auditEventView
}

type consentView struct {
	Connector            string
	OfferAcceptedAt      string
	OfferDocumentLabel   string
	PrivacyAcceptedAt    string
	PrivacyDocumentLabel string
}

type recurringConsentView struct {
	Connector             string
	AcceptedAt            string
	OfferDocumentLabel    string
	UserAgreementDocLabel string
}

type recurringSummaryView struct {
	StatusLabel          string
	StatusClass          string
	LastConsentAt        string
	LastConsentConnector string
	HealthLabel          string
	HealthClass          string
	LastRebillLabel      string
	LastRebillClass      string
	LastRebillAt         string
	FailedAttempts       int
}

type churnIssueView struct {
	UserID             int64
	DisplayName        string
	PrimaryAccount     string
	FullName           string
	Email              string
	Phone              string
	ConnectorID        int64
	Connector          string
	IssueType          string
	IssueLabel         string
	IssueClass         string
	AutoPayLabel       string
	AutoPayClass       string
	RetryLabel         string
	RetryClass         string
	LastRetryAt        string
	PaymentStatus      string
	PaymentLabel       string
	PaymentClass       string
	SubscriptionID     int64
	SubscriptionStatus string
	SubscriptionLabel  string
	SubscriptionClass  string
	LastAmountRUB      int64
	LastEventAt        string
	UserDetailURL      string
	CanSendPayLink     bool
	PaymentLinkURL     string
	CanTriggerRebill   bool
	RebillURL          string
}

type churnPageData struct {
	basePageData

	Notice      string
	ExportURL   string
	UserID      string
	TelegramID  string
	ConnectorID string
	Search      string
	IssueType   string
	AutoPay     string
	RetryState  string
	Issues      []churnIssueView
}

type legalDocumentView struct {
	ID             int64
	Type           string
	TypeLabel      string
	Title          string
	ContentPreview string
	ExternalURL    string
	Version        int
	IsActive       bool
	ActiveLabel    string
	ActiveClass    string
	CreatedAt      string
	PublicURL      string
	ToggleTo       bool
	ToggleLabel    string
	DeleteURL      string
}

type legalDocumentsPageData struct {
	basePageData

	Notice             string
	ExportURL          string
	OfferPublicURL     string
	PrivacyPublicURL   string
	AgreementPublicURL string
	Documents          []legalDocumentView
	EditingID          int64
	Editing            bool
	FormAction         string
	FormSubmitLabel    string
	FormType           string
	FormTitle          string
	FormExternalURL    string
	FormContent        string
	FormIsActive       bool
}

type adminSessionView struct {
	ID          int64
	Subject     string
	IP          string
	UserAgent   string
	CreatedAt   string
	ExpiresAt   string
	LastSeenAt  string
	StatusLabel string
	StatusClass string
	IsCurrent   bool
	CanRevoke   bool
	RevokeURL   string
}

type adminSessionsPageData struct {
	basePageData

	Notice   string
	Sessions []adminSessionView
}
