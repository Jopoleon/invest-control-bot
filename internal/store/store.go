package store

import (
	"context"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
)

// Store describes persistence operations required by bot/admin flows.
type Store interface {
	CreateConnector(ctx context.Context, c domain.Connector) error
	ListConnectors(ctx context.Context) ([]domain.Connector, error)
	GetConnector(ctx context.Context, connectorID string) (domain.Connector, bool, error)
	GetConnectorByStartPayload(ctx context.Context, payload string) (domain.Connector, bool, error)
	SetConnectorActive(ctx context.Context, connectorID string, active bool) error

	SaveConsent(ctx context.Context, consent domain.Consent) error
	GetConsent(ctx context.Context, telegramID int64, connectorID string) (domain.Consent, bool, error)

	SaveUser(ctx context.Context, user domain.User) error
	GetUser(ctx context.Context, telegramID int64) (domain.User, bool, error)

	SaveRegistrationState(ctx context.Context, state domain.RegistrationState) error
	GetRegistrationState(ctx context.Context, telegramID int64) (domain.RegistrationState, bool, error)
	DeleteRegistrationState(ctx context.Context, telegramID int64) error
}
