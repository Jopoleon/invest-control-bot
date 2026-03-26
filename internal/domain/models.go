package domain

import "time"

// MessengerKind identifies an external messenger platform linked to a user.
type MessengerKind string

const (
	MessengerKindTelegram MessengerKind = "telegram"
	MessengerKindMAX      MessengerKind = "max"
)

// Connector describes a tariff and start payload used to enter the bot flow.
type Connector struct {
	ID           int64
	StartPayload string
	Name         string
	Description  string
	ChatID       string
	ChannelURL   string
	PriceRUB     int64
	PeriodDays   int
	OfferURL     string
	PrivacyURL   string
	IsActive     bool
	CreatedAt    time.Time
}

// User stores user profile fields collected during onboarding.
type User struct {
	ID               int64
	TelegramID       int64
	TelegramUsername string
	FullName         string
	Phone            string
	Email            string
	UpdatedAt        time.Time
}

// UserMessengerAccount links one internal user to one external messenger identity.
type UserMessengerAccount struct {
	UserID         int64
	MessengerKind  MessengerKind
	ExternalUserID string
	Username       string
	LinkedAt       time.Time
	UpdatedAt      time.Time
}

// UserListItem is an admin-facing projection for user list screens.
type UserListItem struct {
	UserID             int64
	TelegramID         int64
	TelegramUsername   string
	FullName           string
	Phone              string
	Email              string
	AutoPayEnabled     bool
	HasAutoPaySettings bool
	UpdatedAt          time.Time
}

// UserListQuery describes admin filters for user list.
type UserListQuery struct {
	TelegramID int64
	Search     string
	Limit      int
}

// Consent stores acceptance metadata for offer/privacy terms.
type Consent struct {
	TelegramID             int64
	ConnectorID            int64
	OfferAcceptedAt        time.Time
	PrivacyAcceptedAt      time.Time
	OfferDocumentID        int64
	OfferDocumentVersion   int
	PrivacyDocumentID      int64
	PrivacyDocumentVersion int
}

// LegalDocumentType identifies the kind of legal document exposed to users.
type LegalDocumentType string

const (
	LegalDocumentTypeOffer         LegalDocumentType = "offer"
	LegalDocumentTypePrivacy       LegalDocumentType = "privacy"
	LegalDocumentTypeUserAgreement LegalDocumentType = "user_agreement"
)

// LegalDocument stores versioned legal text or external URL used in onboarding.
type LegalDocument struct {
	ID          int64
	Type        LegalDocumentType
	Title       string
	Content     string
	ExternalURL string
	Version     int
	IsActive    bool
	CreatedAt   time.Time
}

// RecurringConsent stores explicit opt-in history for recurring/autopay charges.
type RecurringConsent struct {
	ID                           int64
	TelegramID                   int64
	ConnectorID                  int64
	AcceptedAt                   time.Time
	OfferDocumentID              int64
	OfferDocumentVersion         int
	UserAgreementDocumentID      int64
	UserAgreementDocumentVersion int
}

// AdminSession stores server-side browser session for admin panel access.
type AdminSession struct {
	ID             int64
	TokenHash      string
	Subject        string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	LastSeenAt     time.Time
	RevokedAt      *time.Time
	IP             string
	UserAgent      string
	RotatedAt      *time.Time
	ReplacedByHash string
}

// RegistrationStep describes current onboarding step in FSM.
type RegistrationStep string

const (
	// FSM states for registration flow.
	StepNone     RegistrationStep = "none"
	StepFullName RegistrationStep = "full_name"
	StepPhone    RegistrationStep = "phone"
	StepEmail    RegistrationStep = "email"
	StepUsername RegistrationStep = "username"
	StepDone     RegistrationStep = "done"
)

// RegistrationState stores in-progress registration context for user.
type RegistrationState struct {
	TelegramID       int64
	ConnectorID      int64
	Step             RegistrationStep
	TelegramUsername string
	UpdatedAt        time.Time
}

// AuditEvent stores immutable user action records for compliance and support.
type AuditEvent struct {
	ID          int64
	TelegramID  int64
	ConnectorID int64
	Action      string
	Details     string
	CreatedAt   time.Time
}

// AuditEventListQuery describes filtering, sorting and pagination for audit event history.
type AuditEventListQuery struct {
	TelegramID int64

	ConnectorID int64
	Action      string
	Search      string

	CreatedFrom      *time.Time
	CreatedToExclude *time.Time

	SortBy   string
	SortDesc bool

	Page     int
	PageSize int
}

// PaymentStatus is lifecycle state of payment transaction.
type PaymentStatus string

const (
	PaymentStatusPending PaymentStatus = "pending"
	PaymentStatusPaid    PaymentStatus = "paid"
	PaymentStatusFailed  PaymentStatus = "failed"
)

// Payment stores provider checkout attempt and final state.
type Payment struct {
	ID                int64
	Provider          string
	ProviderPaymentID string
	Status            PaymentStatus
	Token             string
	TelegramID        int64
	ConnectorID       int64
	SubscriptionID    int64
	ParentPaymentID   int64
	AmountRUB         int64
	AutoPayEnabled    bool
	CheckoutURL       string
	CreatedAt         time.Time
	PaidAt            *time.Time
	UpdatedAt         time.Time
}

// SubscriptionStatus is lifecycle state for user access period.
type SubscriptionStatus string

const (
	SubscriptionStatusActive  SubscriptionStatus = "active"
	SubscriptionStatusExpired SubscriptionStatus = "expired"
	SubscriptionStatusRevoked SubscriptionStatus = "revoked"
)

// Subscription stores purchased access period linked to successful payment.
type Subscription struct {
	ID                 int64
	TelegramID         int64
	ConnectorID        int64
	PaymentID          int64
	Status             SubscriptionStatus
	AutoPayEnabled     bool
	StartsAt           time.Time
	EndsAt             time.Time
	ReminderSentAt     *time.Time
	ExpiryNoticeSentAt *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// PaymentListQuery describes admin filters for payment list.
type PaymentListQuery struct {
	TelegramID int64

	ConnectorID int64
	Status      PaymentStatus

	CreatedFrom      *time.Time
	CreatedToExclude *time.Time

	Limit int
}

// SubscriptionListQuery describes admin filters for subscription list.
type SubscriptionListQuery struct {
	TelegramID int64

	ConnectorID int64
	Status      SubscriptionStatus

	CreatedFrom      *time.Time
	CreatedToExclude *time.Time

	Limit int
}
