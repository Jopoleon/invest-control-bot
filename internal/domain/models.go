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
	ID           int64     `db:"id" json:"id"`
	StartPayload string    `db:"start_payload" json:"start_payload"`
	Name         string    `db:"name" json:"name"`
	Description  string    `db:"description" json:"description"`
	ChatID       string    `db:"chat_id" json:"chat_id"`
	ChannelURL   string    `db:"channel_url" json:"channel_url"`
	PriceRUB     int64     `db:"price_rub" json:"price_rub"`
	PeriodDays   int       `db:"period_days" json:"period_days"`
	OfferURL     string    `db:"offer_url" json:"offer_url"`
	PrivacyURL   string    `db:"privacy_url" json:"privacy_url"`
	IsActive     bool      `db:"is_active" json:"is_active"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

// User stores user profile fields collected during onboarding.
type User struct {
	ID        int64     `db:"id" json:"id"`
	FullName  string    `db:"full_name" json:"full_name"`
	Phone     string    `db:"phone" json:"phone"`
	Email     string    `db:"email" json:"email"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// UserMessengerAccount links one internal user to one external messenger identity.
type UserMessengerAccount struct {
	UserID          int64         `db:"user_id" json:"user_id"`
	MessengerKind   MessengerKind `db:"messenger_kind" json:"messenger_kind"`
	MessengerUserID string        `db:"messenger_user_id" json:"messenger_user_id"`
	Username        string        `db:"username" json:"username"`
	LinkedAt        time.Time     `db:"linked_at" json:"linked_at"`
	UpdatedAt       time.Time     `db:"updated_at" json:"updated_at"`
}

// UserListItem is an admin-facing projection for user list screens.
type UserListItem struct {
	UserID             int64     `db:"user_id" json:"user_id"`
	TelegramID         int64     `db:"telegram_id" json:"telegram_id"`
	TelegramUsername   string    `db:"telegram_username" json:"telegram_username"`
	FullName           string    `db:"full_name" json:"full_name"`
	Phone              string    `db:"phone" json:"phone"`
	Email              string    `db:"email" json:"email"`
	AutoPayEnabled     bool      `db:"auto_pay_enabled" json:"auto_pay_enabled"`
	HasAutoPaySettings bool      `db:"has_auto_pay_settings" json:"has_auto_pay_settings"`
	UpdatedAt          time.Time `db:"updated_at" json:"updated_at"`
}

// UserListQuery describes admin filters for user list.
type UserListQuery struct {
	TelegramID int64  `json:"telegram_id"`
	Search     string `json:"search"`
	Limit      int    `json:"limit"`
}

// Consent stores acceptance metadata for offer/privacy terms.
type Consent struct {
	UserID                 int64     `db:"user_id" json:"user_id"`
	ConnectorID            int64     `db:"connector_id" json:"connector_id"`
	OfferAcceptedAt        time.Time `db:"offer_accepted_at" json:"offer_accepted_at"`
	PrivacyAcceptedAt      time.Time `db:"privacy_accepted_at" json:"privacy_accepted_at"`
	OfferDocumentID        int64     `db:"offer_document_id" json:"offer_document_id"`
	OfferDocumentVersion   int       `db:"offer_document_version" json:"offer_document_version"`
	PrivacyDocumentID      int64     `db:"privacy_document_id" json:"privacy_document_id"`
	PrivacyDocumentVersion int       `db:"privacy_document_version" json:"privacy_document_version"`
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
	ID          int64             `db:"id" json:"id"`
	Type        LegalDocumentType `db:"doc_type" json:"type"`
	Title       string            `db:"title" json:"title"`
	Content     string            `db:"content" json:"content"`
	ExternalURL string            `db:"external_url" json:"external_url"`
	Version     int               `db:"version" json:"version"`
	IsActive    bool              `db:"is_active" json:"is_active"`
	CreatedAt   time.Time         `db:"created_at" json:"created_at"`
}

// RecurringConsent stores explicit opt-in history for recurring/autopay charges.
type RecurringConsent struct {
	ID                           int64     `db:"id" json:"id"`
	UserID                       int64     `db:"user_id" json:"user_id"`
	ConnectorID                  int64     `db:"connector_id" json:"connector_id"`
	AcceptedAt                   time.Time `db:"accepted_at" json:"accepted_at"`
	OfferDocumentID              int64     `db:"offer_document_id" json:"offer_document_id"`
	OfferDocumentVersion         int       `db:"offer_document_version" json:"offer_document_version"`
	UserAgreementDocumentID      int64     `db:"user_agreement_document_id" json:"user_agreement_document_id"`
	UserAgreementDocumentVersion int       `db:"user_agreement_document_version" json:"user_agreement_document_version"`
}

// AdminSession stores server-side browser session for admin panel access.
type AdminSession struct {
	ID             int64      `db:"id" json:"id"`
	TokenHash      string     `db:"session_token_hash" json:"token_hash"`
	Subject        string     `db:"subject" json:"subject"`
	CreatedAt      time.Time  `db:"created_at" json:"created_at"`
	ExpiresAt      time.Time  `db:"expires_at" json:"expires_at"`
	LastSeenAt     time.Time  `db:"last_seen_at" json:"last_seen_at"`
	RevokedAt      *time.Time `db:"revoked_at" json:"revoked_at,omitempty"`
	IP             string     `db:"ip" json:"ip"`
	UserAgent      string     `db:"user_agent" json:"user_agent"`
	RotatedAt      *time.Time `db:"rotated_at" json:"rotated_at,omitempty"`
	ReplacedByHash string     `db:"replaced_by_hash" json:"replaced_by_hash"`
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
	MessengerKind   MessengerKind    `db:"messenger_kind" json:"messenger_kind"`
	MessengerUserID string           `db:"messenger_user_id" json:"messenger_user_id"`
	ConnectorID     int64            `db:"connector_id" json:"connector_id"`
	Step            RegistrationStep `db:"step" json:"step"`
	Username        string           `db:"username" json:"username"`
	UpdatedAt       time.Time        `db:"updated_at" json:"updated_at"`
}

// AuditActorType identifies the source of the audited action.
type AuditActorType string

const (
	AuditActorTypeUser  AuditActorType = "user"
	AuditActorTypeAdmin AuditActorType = "admin"
	AuditActorTypeApp   AuditActorType = "app"
)

// AuditEvent stores immutable audit records with explicit actor and target context.
type AuditEvent struct {
	ID                    int64          `db:"id" json:"id"`
	ActorType             AuditActorType `db:"actor_type" json:"actor_type"`
	ActorUserID           int64          `db:"actor_user_id" json:"actor_user_id"`
	ActorMessengerKind    MessengerKind  `db:"actor_messenger_kind" json:"actor_messenger_kind"`
	ActorMessengerUserID  string         `db:"actor_messenger_user_id" json:"actor_messenger_user_id"`
	ActorSubject          string         `db:"actor_subject" json:"actor_subject"`
	TargetUserID          int64          `db:"target_user_id" json:"target_user_id"`
	TargetMessengerKind   MessengerKind  `db:"target_messenger_kind" json:"target_messenger_kind"`
	TargetMessengerUserID string         `db:"target_messenger_user_id" json:"target_messenger_user_id"`
	ConnectorID           int64          `db:"connector_id" json:"connector_id"`
	Action                string         `db:"action" json:"action"`
	Details               string         `db:"details" json:"details"`
	CreatedAt             time.Time      `db:"created_at" json:"created_at"`
}

// AuditEventListQuery describes filtering, sorting and pagination for audit event history.
type AuditEventListQuery struct {
	ActorType             AuditActorType `json:"actor_type"`
	TargetUserID          int64          `json:"target_user_id"`
	TargetMessengerKind   MessengerKind  `json:"target_messenger_kind"`
	TargetMessengerUserID string         `json:"target_messenger_user_id"`
	ConnectorID           int64          `json:"connector_id"`
	Action                string         `json:"action"`
	Search                string         `json:"search"`

	CreatedFrom      *time.Time `json:"created_from,omitempty"`
	CreatedToExclude *time.Time `json:"created_to_exclude,omitempty"`

	SortBy   string `json:"sort_by"`
	SortDesc bool   `json:"sort_desc"`

	Page     int `json:"page"`
	PageSize int `json:"page_size"`
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
	ID                int64         `db:"id" json:"id"`
	Provider          string        `db:"provider" json:"provider"`
	ProviderPaymentID string        `db:"provider_payment_id" json:"provider_payment_id"`
	Status            PaymentStatus `db:"status" json:"status"`
	Token             string        `db:"token" json:"token"`
	UserID            int64         `db:"user_id" json:"user_id"`
	TelegramID        int64         `db:"telegram_id" json:"telegram_id"`
	ConnectorID       int64         `db:"connector_id" json:"connector_id"`
	SubscriptionID    int64         `db:"subscription_id" json:"subscription_id"`
	ParentPaymentID   int64         `db:"parent_payment_id" json:"parent_payment_id"`
	AmountRUB         int64         `db:"amount_rub" json:"amount_rub"`
	AutoPayEnabled    bool          `db:"auto_pay_enabled" json:"auto_pay_enabled"`
	CheckoutURL       string        `db:"checkout_url" json:"checkout_url"`
	CreatedAt         time.Time     `db:"created_at" json:"created_at"`
	PaidAt            *time.Time    `db:"paid_at" json:"paid_at,omitempty"`
	UpdatedAt         time.Time     `db:"updated_at" json:"updated_at"`
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
	ID                 int64              `db:"id" json:"id"`
	UserID             int64              `db:"user_id" json:"user_id"`
	TelegramID         int64              `db:"telegram_id" json:"telegram_id"`
	ConnectorID        int64              `db:"connector_id" json:"connector_id"`
	PaymentID          int64              `db:"payment_id" json:"payment_id"`
	Status             SubscriptionStatus `db:"status" json:"status"`
	AutoPayEnabled     bool               `db:"auto_pay_enabled" json:"auto_pay_enabled"`
	StartsAt           time.Time          `db:"starts_at" json:"starts_at"`
	EndsAt             time.Time          `db:"ends_at" json:"ends_at"`
	ReminderSentAt     *time.Time         `db:"reminder_sent_at" json:"reminder_sent_at,omitempty"`
	ExpiryNoticeSentAt *time.Time         `db:"expiry_notice_sent_at" json:"expiry_notice_sent_at,omitempty"`
	CreatedAt          time.Time          `db:"created_at" json:"created_at"`
	UpdatedAt          time.Time          `db:"updated_at" json:"updated_at"`
}

// PaymentListQuery describes admin filters for payment list.
type PaymentListQuery struct {
	UserID     int64 `json:"user_id"`
	TelegramID int64 `json:"telegram_id"`

	ConnectorID int64         `json:"connector_id"`
	Status      PaymentStatus `json:"status"`

	CreatedFrom      *time.Time `json:"created_from,omitempty"`
	CreatedToExclude *time.Time `json:"created_to_exclude,omitempty"`

	Limit int `json:"limit"`
}

// SubscriptionListQuery describes admin filters for subscription list.
type SubscriptionListQuery struct {
	UserID     int64 `json:"user_id"`
	TelegramID int64 `json:"telegram_id"`

	ConnectorID int64              `json:"connector_id"`
	Status      SubscriptionStatus `json:"status"`

	CreatedFrom      *time.Time `json:"created_from,omitempty"`
	CreatedToExclude *time.Time `json:"created_to_exclude,omitempty"`

	Limit int `json:"limit"`
}
