package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (s *Store) CreatePayment(ctx context.Context, payment domain.Payment) error {
	now := time.Now().UTC()
	if payment.CreatedAt.IsZero() {
		payment.CreatedAt = now
	}
	if payment.UpdatedAt.IsZero() {
		payment.UpdatedAt = now
	}
	resolvedUserID, _, err := s.resolveUserIdentity(ctx, payment.UserID, 0)
	if err != nil {
		return err
	}
	payment.UserID = resolvedUserID
	if payment.UserID <= 0 {
		return errors.New("payment user is required")
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO payments (
			provider, provider_payment_id, status, token, user_id, connector_id, subscription_id, parent_payment_id,
			auto_pay_enabled, amount_rub, checkout_url, created_at, paid_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`,
		payment.Provider,
		payment.ProviderPaymentID,
		string(payment.Status),
		payment.Token,
		payment.UserID,
		payment.ConnectorID,
		payment.SubscriptionID,
		payment.ParentPaymentID,
		payment.AutoPayEnabled,
		payment.AmountRUB,
		payment.CheckoutURL,
		payment.CreatedAt,
		payment.PaidAt,
		payment.UpdatedAt,
	)
	return err
}

// GetPaymentByToken returns payment by externally visible checkout token.
func (s *Store) GetPaymentByToken(ctx context.Context, token string) (domain.Payment, bool, error) {
	payment, err := scanPayment(s.db.QueryRowContext(ctx, `
		SELECT p.id, p.provider, p.provider_payment_id, p.status, p.token, COALESCE(p.user_id, 0),
		       p.connector_id, p.subscription_id, p.parent_payment_id,
		       p.auto_pay_enabled, p.amount_rub, p.checkout_url, p.created_at, p.paid_at, p.updated_at
		FROM payments p
		WHERE p.token = $1
	`, token))
	if err == sql.ErrNoRows {
		return domain.Payment{}, false, nil
	}
	if err != nil {
		return domain.Payment{}, false, err
	}
	return payment, true, nil
}

// GetPaymentByID returns payment by internal identifier.
func (s *Store) GetPaymentByID(ctx context.Context, paymentID int64) (domain.Payment, bool, error) {
	payment, err := scanPayment(s.db.QueryRowContext(ctx, `
		SELECT p.id, p.provider, p.provider_payment_id, p.status, p.token, COALESCE(p.user_id, 0),
		       p.connector_id, p.subscription_id, p.parent_payment_id,
		       p.auto_pay_enabled, p.amount_rub, p.checkout_url, p.created_at, p.paid_at, p.updated_at
		FROM payments p
		WHERE p.id = $1
	`, paymentID))
	if err == sql.ErrNoRows {
		return domain.Payment{}, false, nil
	}
	if err != nil {
		return domain.Payment{}, false, err
	}
	return payment, true, nil
}

// GetPendingRebillPaymentBySubscription returns outstanding recurring attempt for subscription.
func (s *Store) GetPendingRebillPaymentBySubscription(ctx context.Context, subscriptionID int64) (domain.Payment, bool, error) {
	if subscriptionID <= 0 {
		return domain.Payment{}, false, nil
	}
	payment, err := scanPayment(s.db.QueryRowContext(ctx, `
		SELECT p.id, p.provider, p.provider_payment_id, p.status, p.token, COALESCE(p.user_id, 0),
		       p.connector_id, p.subscription_id, p.parent_payment_id,
		       p.auto_pay_enabled, p.amount_rub, p.checkout_url, p.created_at, p.paid_at, p.updated_at
		FROM payments p
		WHERE p.subscription_id = $1
		  AND p.status = $2
		ORDER BY p.created_at DESC, p.id DESC
		LIMIT 1
	`, subscriptionID, string(domain.PaymentStatusPending)))
	if err == sql.ErrNoRows {
		return domain.Payment{}, false, nil
	}
	if err != nil {
		return domain.Payment{}, false, err
	}
	return payment, true, nil
}

// UpdatePaymentPaid marks payment as paid and stores provider payment reference.
// Returns true only when row changed from non-paid to paid.
func (s *Store) UpdatePaymentPaid(ctx context.Context, paymentID int64, providerPaymentID string, paidAt time.Time) (bool, error) {
	if paidAt.IsZero() {
		paidAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE payments
		SET status = $2, provider_payment_id = $3, paid_at = $4, updated_at = $5
		WHERE id = $1
		  AND status <> $2
	`, paymentID, string(domain.PaymentStatusPaid), providerPaymentID, paidAt, time.Now().UTC())
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM payments WHERE id = $1)`, paymentID).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, errors.New("payment not found")
	}
	return false, nil
}

// UpdatePaymentFailed marks payment as failed and stores provider reference.
// Returns true only when row changed from non-failed to failed.
func (s *Store) UpdatePaymentFailed(ctx context.Context, paymentID int64, providerPaymentID string, updatedAt time.Time) (bool, error) {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE payments
		SET status = $2, provider_payment_id = $3, updated_at = $4
		WHERE id = $1
		  AND status = $5
	`, paymentID, string(domain.PaymentStatusFailed), providerPaymentID, updatedAt, string(domain.PaymentStatusPending))
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected > 0 {
		return true, nil
	}
	var (
		exists bool
		status string
	)
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM payments WHERE id = $1)`, paymentID).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, errors.New("payment not found")
	}
	if err := s.db.QueryRowContext(ctx, `SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&status); err == nil && status == string(domain.PaymentStatusPaid) {
		return false, nil
	}
	return false, nil
}
