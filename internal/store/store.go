package store

import (
	"context"
	"errors"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

var (
	// ErrConnectorNotFound is returned when connector ID does not exist.
	ErrConnectorNotFound = errors.New("connector not found")
	// ErrConnectorInUse is returned when connector has dependent records and cannot be hard-deleted.
	ErrConnectorInUse = errors.New("connector is in use")
)

// ConnectorStore manages connector catalog persistence.
type ConnectorStore interface {
	CreateConnector(ctx context.Context, c domain.Connector) error
	ListConnectors(ctx context.Context) ([]domain.Connector, error)
	GetConnector(ctx context.Context, connectorID int64) (domain.Connector, bool, error)
	GetConnectorByStartPayload(ctx context.Context, payload string) (domain.Connector, bool, error)
	SetConnectorActive(ctx context.Context, connectorID int64, active bool) error
	DeleteConnector(ctx context.Context, connectorID int64) error
}

// LegalDocumentStore manages legal document registry persistence.
type LegalDocumentStore interface {
	CreateLegalDocument(ctx context.Context, doc domain.LegalDocument) error
	UpdateLegalDocument(ctx context.Context, doc domain.LegalDocument) error
	ListLegalDocuments(ctx context.Context, docType domain.LegalDocumentType) ([]domain.LegalDocument, error)
	GetLegalDocument(ctx context.Context, documentID int64) (domain.LegalDocument, bool, error)
	GetActiveLegalDocument(ctx context.Context, docType domain.LegalDocumentType) (domain.LegalDocument, bool, error)
	SetLegalDocumentActive(ctx context.Context, documentID int64, active bool) error
	DeleteLegalDocument(ctx context.Context, documentID int64) error
}

// AdminSessionStore manages admin auth sessions.
type AdminSessionStore interface {
	CreateAdminSession(ctx context.Context, session domain.AdminSession) error
	ListAdminSessions(ctx context.Context, limit int) ([]domain.AdminSession, error)
	GetAdminSessionByTokenHash(ctx context.Context, tokenHash string) (domain.AdminSession, bool, error)
	TouchAdminSession(ctx context.Context, sessionID int64, lastSeenAt time.Time) error
	RotateAdminSession(ctx context.Context, sessionID int64, newTokenHash string, rotatedAt time.Time) error
	RevokeAdminSession(ctx context.Context, sessionID int64, revokedAt time.Time) error
	CleanupAdminSessions(ctx context.Context, expiredBefore time.Time) error
}

// ConsentStore manages legal and recurring consent persistence.
type ConsentStore interface {
	SaveConsent(ctx context.Context, consent domain.Consent) error
	GetConsent(ctx context.Context, userID int64, connectorID int64) (domain.Consent, bool, error)
	ListConsentsByUser(ctx context.Context, userID int64) ([]domain.Consent, error)
	CreateRecurringConsent(ctx context.Context, consent domain.RecurringConsent) error
	ListRecurringConsentsByUser(ctx context.Context, userID int64) ([]domain.RecurringConsent, error)
}

// UserStore manages user profile and linked messenger identities.
type UserStore interface {
	SaveUser(ctx context.Context, user domain.User) error
	GetUserByID(ctx context.Context, userID int64) (domain.User, bool, error)
	GetUser(ctx context.Context, telegramID int64) (domain.User, bool, error)
	GetUserByMessenger(ctx context.Context, kind domain.MessengerKind, messengerUserID string) (domain.User, bool, error)
	GetOrCreateUserByMessenger(ctx context.Context, kind domain.MessengerKind, messengerUserID, username string) (domain.User, bool, error)
	ListUserMessengerAccounts(ctx context.Context, userID int64) ([]domain.UserMessengerAccount, error)
	ListUsers(ctx context.Context, query domain.UserListQuery) ([]domain.UserListItem, error)
}

// RegistrationStateStore manages bot registration FSM state.
type RegistrationStateStore interface {
	SaveRegistrationState(ctx context.Context, state domain.RegistrationState) error
	GetRegistrationState(ctx context.Context, kind domain.MessengerKind, messengerUserID string) (domain.RegistrationState, bool, error)
	DeleteRegistrationState(ctx context.Context, kind domain.MessengerKind, messengerUserID string) error
}

// AuditEventStore manages immutable audit log persistence.
type AuditEventStore interface {
	SaveAuditEvent(ctx context.Context, event domain.AuditEvent) error
	ListAuditEvents(ctx context.Context, query domain.AuditEventListQuery) ([]domain.AuditEvent, int, error)
}

// PaymentStore manages payment transaction persistence.
type PaymentStore interface {
	CreatePayment(ctx context.Context, payment domain.Payment) error
	GetPaymentByID(ctx context.Context, paymentID int64) (domain.Payment, bool, error)
	GetPaymentByToken(ctx context.Context, token string) (domain.Payment, bool, error)
	GetPendingRebillPaymentBySubscription(ctx context.Context, subscriptionID int64) (domain.Payment, bool, error)
	UpdatePaymentPaid(ctx context.Context, paymentID int64, providerPaymentID string, paidAt time.Time) (bool, error)
	UpdatePaymentFailed(ctx context.Context, paymentID int64, providerPaymentID string, updatedAt time.Time) (bool, error)
	ListPayments(ctx context.Context, query domain.PaymentListQuery) ([]domain.Payment, error)
}

// SubscriptionStore manages subscription lifecycle persistence.
type SubscriptionStore interface {
	UpsertSubscriptionByPayment(ctx context.Context, sub domain.Subscription) error
	GetSubscriptionByID(ctx context.Context, subscriptionID int64) (domain.Subscription, bool, error)
	GetLatestSubscriptionByUserConnector(ctx context.Context, userID, connectorID int64) (domain.Subscription, bool, error)
	ListSubscriptions(ctx context.Context, query domain.SubscriptionListQuery) ([]domain.Subscription, error)
	ListSubscriptionsForReminder(ctx context.Context, remindBefore time.Time, limit int) ([]domain.Subscription, error)
	MarkSubscriptionReminderSent(ctx context.Context, subscriptionID int64, sentAt time.Time) error
	ListSubscriptionsForExpiryNotice(ctx context.Context, noticeBefore time.Time, limit int) ([]domain.Subscription, error)
	MarkSubscriptionExpiryNoticeSent(ctx context.Context, subscriptionID int64, sentAt time.Time) error
	ListExpiredActiveSubscriptions(ctx context.Context, now time.Time, limit int) ([]domain.Subscription, error)
	DisableAutoPayForActiveSubscriptions(ctx context.Context, userID int64, updatedAt time.Time) error
	SetSubscriptionAutoPayEnabled(ctx context.Context, subscriptionID int64, enabled bool, updatedAt time.Time) error
	UpdateSubscriptionStatus(ctx context.Context, subscriptionID int64, status domain.SubscriptionStatus, updatedAt time.Time) error
}

// Store describes persistence operations required by bot/admin flows.
type Store interface {
	ConnectorStore
	LegalDocumentStore
	AdminSessionStore
	ConsentStore
	UserStore
	RegistrationStateStore
	AuditEventStore
	PaymentStore
	SubscriptionStore
}
