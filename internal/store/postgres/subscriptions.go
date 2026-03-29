package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (s *Store) UpsertSubscriptionByPayment(ctx context.Context, sub domain.Subscription) error {
	now := time.Now().UTC()
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	if sub.UpdatedAt.IsZero() {
		sub.UpdatedAt = now
	}
	resolvedUserID, _, err := s.resolveUserIdentity(ctx, sub.UserID, 0)
	if err != nil {
		return err
	}
	sub.UserID = resolvedUserID
	if sub.UserID <= 0 {
		return errors.New("subscription user is required")
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO subscriptions (
			user_id, connector_id, payment_id, status, auto_pay_enabled, starts_at, ends_at, reminder_sent_at, expiry_notice_sent_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (payment_id)
		DO UPDATE SET
			user_id = EXCLUDED.user_id,
			status = EXCLUDED.status,
			auto_pay_enabled = EXCLUDED.auto_pay_enabled,
			starts_at = EXCLUDED.starts_at,
			ends_at = EXCLUDED.ends_at,
			reminder_sent_at = EXCLUDED.reminder_sent_at,
			expiry_notice_sent_at = EXCLUDED.expiry_notice_sent_at,
			updated_at = EXCLUDED.updated_at
	`,
		sub.UserID,
		sub.ConnectorID,
		sub.PaymentID,
		string(sub.Status),
		sub.AutoPayEnabled,
		sub.StartsAt,
		sub.EndsAt,
		sub.ReminderSentAt,
		sub.ExpiryNoticeSentAt,
		sub.CreatedAt,
		sub.UpdatedAt,
	)
	return err
}

// GetSubscriptionByID fetches subscription by internal identifier.
func (s *Store) GetSubscriptionByID(ctx context.Context, subscriptionID int64) (domain.Subscription, bool, error) {
	item, err := scanSubscription(s.db.QueryRowContext(ctx, `
		SELECT s.id, COALESCE(s.user_id, 0),
		       s.connector_id, s.payment_id, s.status, s.auto_pay_enabled, s.starts_at, s.ends_at,
		       s.reminder_sent_at, s.expiry_notice_sent_at, s.created_at, s.updated_at
		FROM subscriptions s
		WHERE s.id = $1
	`, subscriptionID))
	if err == sql.ErrNoRows {
		return domain.Subscription{}, false, nil
	}
	if err != nil {
		return domain.Subscription{}, false, err
	}
	return item, true, nil
}

// GetLatestSubscriptionByUserConnector returns latest subscription by ends_at for user/connector pair.
func (s *Store) GetLatestSubscriptionByUserConnector(ctx context.Context, userID, connectorID int64) (domain.Subscription, bool, error) {
	item, err := scanSubscription(s.db.QueryRowContext(ctx, `
		SELECT s.id, COALESCE(s.user_id, 0),
		       s.connector_id, s.payment_id, s.status, s.auto_pay_enabled, s.starts_at, s.ends_at,
		       s.reminder_sent_at, s.expiry_notice_sent_at, s.created_at, s.updated_at
		FROM subscriptions s
		WHERE s.user_id = $1 AND s.connector_id = $2
		ORDER BY s.ends_at DESC, s.id DESC
		LIMIT 1
	`, userID, connectorID))
	if err == sql.ErrNoRows {
		return domain.Subscription{}, false, nil
	}
	if err != nil {
		return domain.Subscription{}, false, err
	}
	return item, true, nil
}

// ListPayments returns recent payments with optional admin filters.
func (s *Store) ListPayments(ctx context.Context, query domain.PaymentListQuery) ([]domain.Payment, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	where := make([]string, 0, 6)
	args := make([]any, 0, 8)
	where = append(where, "1=1")
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if query.UserID > 0 {
		where = append(where, "p.user_id = "+addArg(query.UserID))
	}
	if query.ConnectorID > 0 {
		where = append(where, "p.connector_id = "+addArg(query.ConnectorID))
	}
	if query.Status != "" {
		where = append(where, "p.status = "+addArg(string(query.Status)))
	}
	if query.CreatedFrom != nil {
		where = append(where, "p.created_at >= "+addArg(*query.CreatedFrom))
	}
	if query.CreatedToExclude != nil {
		where = append(where, "p.created_at < "+addArg(*query.CreatedToExclude))
	}
	whereClause := strings.Join(where, " AND ")
	sqlText := fmt.Sprintf(`
		SELECT p.id, p.provider, p.provider_payment_id, p.status, p.token, COALESCE(p.user_id, 0),
		       p.connector_id, p.subscription_id, p.parent_payment_id,
		       p.auto_pay_enabled, p.amount_rub, p.checkout_url, p.created_at, p.paid_at, p.updated_at
		FROM payments p
		WHERE %s
		ORDER BY p.created_at DESC, p.id DESC
		LIMIT %s
	`, whereClause, addArg(limit))

	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.Payment, 0, limit)
	for rows.Next() {
		item, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// ListSubscriptions returns recent subscriptions with optional admin filters.
func (s *Store) ListSubscriptions(ctx context.Context, query domain.SubscriptionListQuery) ([]domain.Subscription, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	where := make([]string, 0, 6)
	args := make([]any, 0, 8)
	where = append(where, "1=1")
	addArg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if query.UserID > 0 {
		where = append(where, "s.user_id = "+addArg(query.UserID))
	}
	if query.ConnectorID > 0 {
		where = append(where, "s.connector_id = "+addArg(query.ConnectorID))
	}
	if query.Status != "" {
		where = append(where, "s.status = "+addArg(string(query.Status)))
	}
	if query.CreatedFrom != nil {
		where = append(where, "s.created_at >= "+addArg(*query.CreatedFrom))
	}
	if query.CreatedToExclude != nil {
		where = append(where, "s.created_at < "+addArg(*query.CreatedToExclude))
	}
	whereClause := strings.Join(where, " AND ")
	sqlText := fmt.Sprintf(`
		SELECT s.id, COALESCE(s.user_id, 0),
		       s.connector_id, s.payment_id, s.status, s.auto_pay_enabled, s.starts_at, s.ends_at,
		       s.reminder_sent_at, s.expiry_notice_sent_at, s.created_at, s.updated_at
		FROM subscriptions s
		WHERE %s
		ORDER BY s.created_at DESC, s.id DESC
		LIMIT %s
	`, whereClause, addArg(limit))

	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.Subscription, 0, limit)
	for rows.Next() {
		item, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// ListSubscriptionsForReminder returns active subscriptions that need a pre-expiration reminder.
func (s *Store) ListSubscriptionsForReminder(ctx context.Context, remindBefore time.Time, limit int) ([]domain.Subscription, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	now := time.Now().UTC()
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, COALESCE(s.user_id, 0),
		       s.connector_id, s.payment_id, s.status, s.auto_pay_enabled, s.starts_at, s.ends_at,
		       s.reminder_sent_at, s.expiry_notice_sent_at, s.created_at, s.updated_at
		FROM subscriptions s
		WHERE s.status = $1
		  AND s.reminder_sent_at IS NULL
		  AND s.ends_at > $2
		  AND s.ends_at <= $3
		ORDER BY s.ends_at ASC
		LIMIT $4
	`, string(domain.SubscriptionStatusActive), now, remindBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Subscription, 0, limit)
	for rows.Next() {
		item, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// MarkSubscriptionReminderSent stores timestamp when reminder was delivered.
func (s *Store) MarkSubscriptionReminderSent(ctx context.Context, subscriptionID int64, sentAt time.Time) error {
	if sentAt.IsZero() {
		sentAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE subscriptions
		SET reminder_sent_at = $2, updated_at = $3
		WHERE id = $1
	`, subscriptionID, sentAt, time.Now().UTC())
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("subscription not found")
	}
	return nil
}

// ListSubscriptionsForExpiryNotice returns active subscriptions that should receive same-day notice.
func (s *Store) ListSubscriptionsForExpiryNotice(ctx context.Context, noticeBefore time.Time, limit int) ([]domain.Subscription, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	now := time.Now().UTC()
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, COALESCE(s.user_id, 0),
		       s.connector_id, s.payment_id, s.status, s.auto_pay_enabled, s.starts_at, s.ends_at,
		       s.reminder_sent_at, s.expiry_notice_sent_at, s.created_at, s.updated_at
		FROM subscriptions s
		WHERE s.status = $1
		  AND s.expiry_notice_sent_at IS NULL
		  AND s.ends_at > $2
		  AND s.ends_at <= $3
		ORDER BY s.ends_at ASC
		LIMIT $4
	`, string(domain.SubscriptionStatusActive), now, noticeBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Subscription, 0, limit)
	for rows.Next() {
		item, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// MarkSubscriptionExpiryNoticeSent stores timestamp when same-day notice was delivered.
func (s *Store) MarkSubscriptionExpiryNoticeSent(ctx context.Context, subscriptionID int64, sentAt time.Time) error {
	if sentAt.IsZero() {
		sentAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE subscriptions
		SET expiry_notice_sent_at = $2, updated_at = $3
		WHERE id = $1
	`, subscriptionID, sentAt, time.Now().UTC())
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("subscription not found")
	}
	return nil
}

// ListExpiredActiveSubscriptions returns active subscriptions that already ended.
func (s *Store) ListExpiredActiveSubscriptions(ctx context.Context, now time.Time, limit int) ([]domain.Subscription, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, COALESCE(s.user_id, 0),
		       s.connector_id, s.payment_id, s.status, s.auto_pay_enabled, s.starts_at, s.ends_at,
		       s.reminder_sent_at, s.expiry_notice_sent_at, s.created_at, s.updated_at
		FROM subscriptions s
		WHERE s.status = $1
		  AND s.ends_at <= $2
		ORDER BY s.ends_at ASC
		LIMIT $3
	`, string(domain.SubscriptionStatusActive), now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Subscription, 0, limit)
	for rows.Next() {
		item, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// UpdateSubscriptionStatus updates subscription status and touched timestamp.
func (s *Store) UpdateSubscriptionStatus(ctx context.Context, subscriptionID int64, status domain.SubscriptionStatus, updatedAt time.Time) error {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE subscriptions
		SET status = $2, updated_at = $3
		WHERE id = $1
	`, subscriptionID, string(status), updatedAt)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("subscription not found")
	}
	return nil
}

// DisableAutoPayForActiveSubscriptions clears recurring flag for all active subscriptions of one user.
func (s *Store) DisableAutoPayForActiveSubscriptions(ctx context.Context, userID int64, updatedAt time.Time) error {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE subscriptions
		SET auto_pay_enabled = false, updated_at = $2
		WHERE user_id = $1
		  AND status = $3
		  AND auto_pay_enabled = true
	`, userID, updatedAt, string(domain.SubscriptionStatusActive))
	return err
}

// SetSubscriptionAutoPayEnabled updates recurring flag for a single subscription.
func (s *Store) SetSubscriptionAutoPayEnabled(ctx context.Context, subscriptionID int64, enabled bool, updatedAt time.Time) error {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE subscriptions
		SET auto_pay_enabled = $2, updated_at = $3
		WHERE id = $1
	`, subscriptionID, enabled, updatedAt)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("subscription not found")
	}
	return nil
}
