package store

import (
	"context"
	"errors"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
)

var (
	// ErrConnectorNotFound is returned when connector ID does not exist.
	ErrConnectorNotFound = errors.New("connector not found")
	// ErrConnectorInUse is returned when connector has dependent records and cannot be hard-deleted.
	ErrConnectorInUse = errors.New("connector is in use")
)

// Store describes persistence operations required by bot/admin flows.
type Store interface {
	CreateConnector(ctx context.Context, c domain.Connector) error
	ListConnectors(ctx context.Context) ([]domain.Connector, error)
	GetConnector(ctx context.Context, connectorID int64) (domain.Connector, bool, error)
	GetConnectorByStartPayload(ctx context.Context, payload string) (domain.Connector, bool, error)
	SetConnectorActive(ctx context.Context, connectorID int64, active bool) error
	DeleteConnector(ctx context.Context, connectorID int64) error

	SaveConsent(ctx context.Context, consent domain.Consent) error
	GetConsent(ctx context.Context, telegramID int64, connectorID int64) (domain.Consent, bool, error)

	SaveUser(ctx context.Context, user domain.User) error
	GetUser(ctx context.Context, telegramID int64) (domain.User, bool, error)
	SetUserAutoPayEnabled(ctx context.Context, telegramID int64, enabled bool, updatedAt time.Time) error
	GetUserAutoPayEnabled(ctx context.Context, telegramID int64) (bool, bool, error)

	SaveRegistrationState(ctx context.Context, state domain.RegistrationState) error
	GetRegistrationState(ctx context.Context, telegramID int64) (domain.RegistrationState, bool, error)
	DeleteRegistrationState(ctx context.Context, telegramID int64) error

	SaveAuditEvent(ctx context.Context, event domain.AuditEvent) error
	ListAuditEvents(ctx context.Context, query domain.AuditEventListQuery) ([]domain.AuditEvent, int, error)

	CreatePayment(ctx context.Context, payment domain.Payment) error
	GetPaymentByID(ctx context.Context, paymentID int64) (domain.Payment, bool, error)
	GetPaymentByToken(ctx context.Context, token string) (domain.Payment, bool, error)
	GetPendingRebillPaymentBySubscription(ctx context.Context, subscriptionID int64) (domain.Payment, bool, error)
	UpdatePaymentPaid(ctx context.Context, paymentID int64, providerPaymentID string, paidAt time.Time) (bool, error)
	UpdatePaymentFailed(ctx context.Context, paymentID int64, providerPaymentID string, updatedAt time.Time) (bool, error)
	ListPayments(ctx context.Context, query domain.PaymentListQuery) ([]domain.Payment, error)

	UpsertSubscriptionByPayment(ctx context.Context, sub domain.Subscription) error
	GetSubscriptionByID(ctx context.Context, subscriptionID int64) (domain.Subscription, bool, error)
	GetLatestSubscriptionByUserConnector(ctx context.Context, telegramID, connectorID int64) (domain.Subscription, bool, error)
	ListSubscriptions(ctx context.Context, query domain.SubscriptionListQuery) ([]domain.Subscription, error)
	ListSubscriptionsForReminder(ctx context.Context, remindBefore time.Time, limit int) ([]domain.Subscription, error)
	MarkSubscriptionReminderSent(ctx context.Context, subscriptionID int64, sentAt time.Time) error
	ListSubscriptionsForExpiryNotice(ctx context.Context, noticeBefore time.Time, limit int) ([]domain.Subscription, error)
	MarkSubscriptionExpiryNoticeSent(ctx context.Context, subscriptionID int64, sentAt time.Time) error
	ListExpiredActiveSubscriptions(ctx context.Context, now time.Time, limit int) ([]domain.Subscription, error)
	UpdateSubscriptionStatus(ctx context.Context, subscriptionID int64, status domain.SubscriptionStatus, updatedAt time.Time) error
}
