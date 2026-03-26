package postgres

import (
	"database/sql"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/jmoiron/sqlx"
)

// Store is a PostgreSQL-backed implementation of store.Store.
type Store struct {
	db *sqlx.DB
}

type rowScanner interface {
	Scan(dest ...any) error
}

// New creates PostgreSQL store from opened sqlx.DB connection.
func New(db *sqlx.DB) *Store {
	return &Store{db: db}
}

func scanPayment(scanner rowScanner) (domain.Payment, error) {
	var (
		payment domain.Payment
		status  string
	)
	err := scanner.Scan(
		&payment.ID,
		&payment.Provider,
		&payment.ProviderPaymentID,
		&status,
		&payment.Token,
		&payment.UserID,
		&payment.TelegramID,
		&payment.ConnectorID,
		&payment.SubscriptionID,
		&payment.ParentPaymentID,
		&payment.AutoPayEnabled,
		&payment.AmountRUB,
		&payment.CheckoutURL,
		&payment.CreatedAt,
		&payment.PaidAt,
		&payment.UpdatedAt,
	)
	if err != nil {
		return domain.Payment{}, err
	}
	payment.Status = domain.PaymentStatus(status)
	return payment, nil
}

func scanSubscription(scanner rowScanner) (domain.Subscription, error) {
	var (
		item   domain.Subscription
		status string
	)
	err := scanner.Scan(
		&item.ID,
		&item.UserID,
		&item.TelegramID,
		&item.ConnectorID,
		&item.PaymentID,
		&status,
		&item.AutoPayEnabled,
		&item.StartsAt,
		&item.EndsAt,
		&item.ReminderSentAt,
		&item.ExpiryNoticeSentAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return domain.Subscription{}, err
	}
	item.Status = domain.SubscriptionStatus(status)
	return item, nil
}

func scanLegalDocument(scanner rowScanner) (domain.LegalDocument, error) {
	var (
		item    domain.LegalDocument
		docType string
	)
	err := scanner.Scan(
		&item.ID,
		&docType,
		&item.Title,
		&item.Content,
		&item.ExternalURL,
		&item.Version,
		&item.IsActive,
		&item.CreatedAt,
	)
	if err != nil {
		return domain.LegalDocument{}, err
	}
	item.Type = domain.LegalDocumentType(docType)
	return item, nil
}

func scanAdminSession(scanner rowScanner) (domain.AdminSession, error) {
	var session domain.AdminSession
	err := scanner.Scan(
		&session.ID,
		&session.TokenHash,
		&session.Subject,
		&session.CreatedAt,
		&session.ExpiresAt,
		&session.LastSeenAt,
		&session.RevokedAt,
		&session.IP,
		&session.UserAgent,
		&session.RotatedAt,
		&session.ReplacedByHash,
	)
	if err != nil {
		return domain.AdminSession{}, err
	}
	return session, nil
}

func nullableInt64(value int64) sql.NullInt64 {
	if value <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: value, Valid: true}
}
