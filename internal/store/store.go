package store

import (
	"context"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
)

// Store describes persistence operations required by bot/admin flows.
type Store interface {
	CreateConnector(ctx context.Context, c domain.Connector) error
	ListConnectors(ctx context.Context) ([]domain.Connector, error)
	GetConnector(ctx context.Context, connectorID int64) (domain.Connector, bool, error)
	GetConnectorByStartPayload(ctx context.Context, payload string) (domain.Connector, bool, error)
	SetConnectorActive(ctx context.Context, connectorID int64, active bool) error

	SaveConsent(ctx context.Context, consent domain.Consent) error
	GetConsent(ctx context.Context, telegramID int64, connectorID int64) (domain.Consent, bool, error)

	SaveUser(ctx context.Context, user domain.User) error
	GetUser(ctx context.Context, telegramID int64) (domain.User, bool, error)

	SaveRegistrationState(ctx context.Context, state domain.RegistrationState) error
	GetRegistrationState(ctx context.Context, telegramID int64) (domain.RegistrationState, bool, error)
	DeleteRegistrationState(ctx context.Context, telegramID int64) error

	SaveAuditEvent(ctx context.Context, event domain.AuditEvent) error
	ListAuditEvents(ctx context.Context, query domain.AuditEventListQuery) ([]domain.AuditEvent, int, error)

	CreatePayment(ctx context.Context, payment domain.Payment) error
	GetPaymentByToken(ctx context.Context, token string) (domain.Payment, bool, error)
	UpdatePaymentPaid(ctx context.Context, paymentID int64, providerPaymentID string, paidAt time.Time) error
	ListPayments(ctx context.Context, query domain.PaymentListQuery) ([]domain.Payment, error)

	UpsertSubscriptionByPayment(ctx context.Context, sub domain.Subscription) error
	ListSubscriptions(ctx context.Context, query domain.SubscriptionListQuery) ([]domain.Subscription, error)
	ListSubscriptionsForReminder(ctx context.Context, remindBefore time.Time, limit int) ([]domain.Subscription, error)
	MarkSubscriptionReminderSent(ctx context.Context, subscriptionID int64, sentAt time.Time) error
	ListExpiredActiveSubscriptions(ctx context.Context, now time.Time, limit int) ([]domain.Subscription, error)
	UpdateSubscriptionStatus(ctx context.Context, subscriptionID int64, status domain.SubscriptionStatus, updatedAt time.Time) error
}
