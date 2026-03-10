package domain

import "time"

// Connector describes a tariff and start payload used to enter the bot flow.
type Connector struct {
	ID           string
	StartPayload string
	Name         string
	Description  string
	ChatID       string
	PriceRUB     int64
	PeriodDays   int
	OfferURL     string
	PrivacyURL   string
	IsActive     bool
	CreatedAt    time.Time
}

// User stores user profile fields collected during onboarding.
type User struct {
	TelegramID       int64
	TelegramUsername string
	FullName         string
	Phone            string
	Email            string
	UpdatedAt        time.Time
}

// Consent stores acceptance metadata for offer/privacy terms.
type Consent struct {
	TelegramID        int64
	ConnectorID       string
	OfferAcceptedAt   time.Time
	PrivacyAcceptedAt time.Time
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
	ConnectorID      string
	Step             RegistrationStep
	TelegramUsername string
	UpdatedAt        time.Time
}
