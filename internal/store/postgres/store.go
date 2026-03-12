package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/jmoiron/sqlx"
)

// Store is a PostgreSQL-backed implementation of store.Store.
type Store struct {
	db *sqlx.DB
}

// New creates PostgreSQL store from opened sqlx.DB connection.
func New(db *sqlx.DB) *Store {
	return &Store{db: db}
}

// CreateConnector inserts connector row.
func (s *Store) CreateConnector(ctx context.Context, c domain.Connector) error {
	if c.StartPayload == "" {
		return errors.New("start payload is required")
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO connectors (
			start_payload, name, description, chat_id, channel_url, price_rub, period_days,
			offer_url, privacy_url, is_active, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`,
		c.StartPayload,
		c.Name,
		c.Description,
		c.ChatID,
		c.ChannelURL,
		c.PriceRUB,
		c.PeriodDays,
		c.OfferURL,
		c.PrivacyURL,
		c.IsActive,
		c.CreatedAt,
	)
	return err
}

// ListConnectors returns connectors ordered by created_at.
func (s *Store) ListConnectors(ctx context.Context) ([]domain.Connector, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, start_payload, name, description, chat_id, channel_url, price_rub, period_days,
		       offer_url, privacy_url, is_active, created_at
		FROM connectors
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.Connector, 0)
	for rows.Next() {
		var c domain.Connector
		if err := rows.Scan(
			&c.ID,
			&c.StartPayload,
			&c.Name,
			&c.Description,
			&c.ChatID,
			&c.ChannelURL,
			&c.PriceRUB,
			&c.PeriodDays,
			&c.OfferURL,
			&c.PrivacyURL,
			&c.IsActive,
			&c.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// GetConnector fetches connector by ID.
func (s *Store) GetConnector(ctx context.Context, connectorID int64) (domain.Connector, bool, error) {
	var c domain.Connector
	err := s.db.QueryRowContext(ctx, `
		SELECT id, start_payload, name, description, chat_id, channel_url, price_rub, period_days,
		       offer_url, privacy_url, is_active, created_at
		FROM connectors
		WHERE id = $1
	`, connectorID).Scan(
		&c.ID,
		&c.StartPayload,
		&c.Name,
		&c.Description,
		&c.ChatID,
		&c.ChannelURL,
		&c.PriceRUB,
		&c.PeriodDays,
		&c.OfferURL,
		&c.PrivacyURL,
		&c.IsActive,
		&c.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return domain.Connector{}, false, nil
	}
	if err != nil {
		return domain.Connector{}, false, err
	}
	return c, true, nil
}

// GetConnectorByStartPayload fetches connector by deeplink payload.
func (s *Store) GetConnectorByStartPayload(ctx context.Context, payload string) (domain.Connector, bool, error) {
	var c domain.Connector
	err := s.db.QueryRowContext(ctx, `
		SELECT id, start_payload, name, description, chat_id, channel_url, price_rub, period_days,
		       offer_url, privacy_url, is_active, created_at
		FROM connectors
		WHERE start_payload = $1
	`, payload).Scan(
		&c.ID,
		&c.StartPayload,
		&c.Name,
		&c.Description,
		&c.ChatID,
		&c.ChannelURL,
		&c.PriceRUB,
		&c.PeriodDays,
		&c.OfferURL,
		&c.PrivacyURL,
		&c.IsActive,
		&c.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return domain.Connector{}, false, nil
	}
	if err != nil {
		return domain.Connector{}, false, err
	}
	return c, true, nil
}

// SetConnectorActive toggles connector active status.
func (s *Store) SetConnectorActive(ctx context.Context, connectorID int64, active bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE connectors SET is_active = $2 WHERE id = $1`, connectorID, active)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("connector not found")
	}
	return nil
}

// SaveConsent upserts user consent for connector.
func (s *Store) SaveConsent(ctx context.Context, consent domain.Consent) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_consents (
			telegram_id, connector_id, offer_accepted_at, privacy_accepted_at
		) VALUES ($1,$2,$3,$4)
		ON CONFLICT (telegram_id, connector_id)
		DO UPDATE SET
			offer_accepted_at = EXCLUDED.offer_accepted_at,
			privacy_accepted_at = EXCLUDED.privacy_accepted_at
	`, consent.TelegramID, consent.ConnectorID, consent.OfferAcceptedAt, consent.PrivacyAcceptedAt)
	return err
}

// GetConsent returns stored consent.
func (s *Store) GetConsent(ctx context.Context, telegramID int64, connectorID int64) (domain.Consent, bool, error) {
	var c domain.Consent
	err := s.db.QueryRowContext(ctx, `
		SELECT telegram_id, connector_id, offer_accepted_at, privacy_accepted_at
		FROM user_consents
		WHERE telegram_id = $1 AND connector_id = $2
	`, telegramID, connectorID).Scan(&c.TelegramID, &c.ConnectorID, &c.OfferAcceptedAt, &c.PrivacyAcceptedAt)
	if err == sql.ErrNoRows {
		return domain.Consent{}, false, nil
	}
	if err != nil {
		return domain.Consent{}, false, err
	}
	return c, true, nil
}

// SaveUser upserts user profile.
func (s *Store) SaveUser(ctx context.Context, user domain.User) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (
			telegram_id, telegram_username, full_name, phone, email, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (telegram_id)
		DO UPDATE SET
			telegram_username = EXCLUDED.telegram_username,
			full_name = EXCLUDED.full_name,
			phone = EXCLUDED.phone,
			email = EXCLUDED.email,
			updated_at = EXCLUDED.updated_at
	`, user.TelegramID, user.TelegramUsername, user.FullName, user.Phone, user.Email, now)
	return err
}

// GetUser fetches user by Telegram ID.
func (s *Store) GetUser(ctx context.Context, telegramID int64) (domain.User, bool, error) {
	var u domain.User
	err := s.db.QueryRowContext(ctx, `
		SELECT telegram_id, telegram_username, full_name, phone, email, updated_at
		FROM users
		WHERE telegram_id = $1
	`, telegramID).Scan(&u.TelegramID, &u.TelegramUsername, &u.FullName, &u.Phone, &u.Email, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return domain.User{}, false, nil
	}
	if err != nil {
		return domain.User{}, false, err
	}
	return u, true, nil
}

// SetUserAutoPayEnabled stores user recurring preference used for future payment creation.
func (s *Store) SetUserAutoPayEnabled(ctx context.Context, telegramID int64, enabled bool, updatedAt time.Time) error {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_settings (telegram_id, auto_pay_enabled, updated_at)
		VALUES ($1,$2,$3)
		ON CONFLICT (telegram_id)
		DO UPDATE SET
			auto_pay_enabled = EXCLUDED.auto_pay_enabled,
			updated_at = EXCLUDED.updated_at
	`, telegramID, enabled, updatedAt)
	return err
}

// GetUserAutoPayEnabled returns user recurring preference if record exists.
func (s *Store) GetUserAutoPayEnabled(ctx context.Context, telegramID int64) (bool, bool, error) {
	var enabled bool
	err := s.db.QueryRowContext(ctx, `
		SELECT auto_pay_enabled
		FROM user_settings
		WHERE telegram_id = $1
	`, telegramID).Scan(&enabled)
	if err == sql.ErrNoRows {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return enabled, true, nil
}

// SaveRegistrationState upserts registration FSM state.
func (s *Store) SaveRegistrationState(ctx context.Context, state domain.RegistrationState) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO registration_states (
			telegram_id, connector_id, step, telegram_username, updated_at
		) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (telegram_id)
		DO UPDATE SET
			connector_id = EXCLUDED.connector_id,
			step = EXCLUDED.step,
			telegram_username = EXCLUDED.telegram_username,
			updated_at = EXCLUDED.updated_at
	`, state.TelegramID, state.ConnectorID, string(state.Step), state.TelegramUsername, now)
	return err
}

// GetRegistrationState fetches registration FSM state.
func (s *Store) GetRegistrationState(ctx context.Context, telegramID int64) (domain.RegistrationState, bool, error) {
	var state domain.RegistrationState
	var step string
	err := s.db.QueryRowContext(ctx, `
		SELECT telegram_id, connector_id, step, telegram_username, updated_at
		FROM registration_states
		WHERE telegram_id = $1
	`, telegramID).Scan(&state.TelegramID, &state.ConnectorID, &step, &state.TelegramUsername, &state.UpdatedAt)
	if err == sql.ErrNoRows {
		return domain.RegistrationState{}, false, nil
	}
	if err != nil {
		return domain.RegistrationState{}, false, err
	}
	state.Step = domain.RegistrationStep(step)
	return state, true, nil
}

// DeleteRegistrationState removes registration FSM state.
func (s *Store) DeleteRegistrationState(ctx context.Context, telegramID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM registration_states WHERE telegram_id = $1`, telegramID)
	return err
}

// SaveAuditEvent writes immutable user action to audit_events table.
func (s *Store) SaveAuditEvent(ctx context.Context, event domain.AuditEvent) error {
	now := time.Now().UTC()
	if !event.CreatedAt.IsZero() {
		now = event.CreatedAt
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_events (
			telegram_id, connector_id, action, details, created_at
		) VALUES ($1,$2,$3,$4,$5)
	`, event.TelegramID, event.ConnectorID, event.Action, event.Details, now)
	return err
}

// ListAuditEvents returns last N events ordered by newest first.
func (s *Store) ListAuditEvents(ctx context.Context, query domain.AuditEventListQuery) ([]domain.AuditEvent, int, error) {
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 50
	}
	if query.PageSize > 500 {
		query.PageSize = 500
	}

	sortColumn := "created_at"
	switch query.SortBy {
	case "telegram_id":
		sortColumn = "telegram_id"
	case "connector_id":
		sortColumn = "connector_id"
	case "action":
		sortColumn = "action"
	case "created_at", "":
		sortColumn = "created_at"
	default:
		sortColumn = "created_at"
	}
	sortDir := "ASC"
	if query.SortDesc {
		sortDir = "DESC"
	}

	where := make([]string, 0, 8)
	args := make([]any, 0, 10)
	where = append(where, "1=1")
	addArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}

	if query.TelegramID > 0 {
		where = append(where, "telegram_id = "+addArg(query.TelegramID))
	}
	if query.ConnectorID > 0 {
		where = append(where, "connector_id = "+addArg(query.ConnectorID))
	}
	if query.Action != "" {
		where = append(where, "action = "+addArg(query.Action))
	}
	if query.Search != "" {
		where = append(where, "details ILIKE "+addArg("%"+query.Search+"%"))
	}
	if query.CreatedFrom != nil {
		where = append(where, "created_at >= "+addArg(*query.CreatedFrom))
	}
	if query.CreatedToExclude != nil {
		where = append(where, "created_at < "+addArg(*query.CreatedToExclude))
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	countSQL := "SELECT COUNT(*) FROM audit_events WHERE " + whereClause
	if err := s.db.QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (query.Page - 1) * query.PageSize
	dataSQL := fmt.Sprintf(`
		SELECT id, telegram_id, connector_id, action, details, created_at
		FROM audit_events
		WHERE %s
		ORDER BY %s %s, id %s
		LIMIT %s OFFSET %s
	`, whereClause, sortColumn, sortDir, sortDir, addArg(query.PageSize), addArg(offset))

	rows, err := s.db.QueryContext(ctx, dataSQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := make([]domain.AuditEvent, 0, query.PageSize)
	for rows.Next() {
		var event domain.AuditEvent
		if err := rows.Scan(
			&event.ID,
			&event.TelegramID,
			&event.ConnectorID,
			&event.Action,
			&event.Details,
			&event.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		result = append(result, event)
	}
	return result, total, rows.Err()
}

// CreatePayment inserts pending payment transaction.
func (s *Store) CreatePayment(ctx context.Context, payment domain.Payment) error {
	now := time.Now().UTC()
	if payment.CreatedAt.IsZero() {
		payment.CreatedAt = now
	}
	if payment.UpdatedAt.IsZero() {
		payment.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO payments (
			provider, provider_payment_id, status, token, telegram_id, connector_id, auto_pay_enabled,
			amount_rub, checkout_url, created_at, paid_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`,
		payment.Provider,
		payment.ProviderPaymentID,
		string(payment.Status),
		payment.Token,
		payment.TelegramID,
		payment.ConnectorID,
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
	var payment domain.Payment
	var status string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, provider, provider_payment_id, status, token, telegram_id, connector_id, auto_pay_enabled,
		       amount_rub, checkout_url, created_at, paid_at, updated_at
		FROM payments
		WHERE token = $1
	`, token).Scan(
		&payment.ID,
		&payment.Provider,
		&payment.ProviderPaymentID,
		&status,
		&payment.Token,
		&payment.TelegramID,
		&payment.ConnectorID,
		&payment.AutoPayEnabled,
		&payment.AmountRUB,
		&payment.CheckoutURL,
		&payment.CreatedAt,
		&payment.PaidAt,
		&payment.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return domain.Payment{}, false, nil
	}
	if err != nil {
		return domain.Payment{}, false, err
	}
	payment.Status = domain.PaymentStatus(status)
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

// UpsertSubscriptionByPayment creates/updates access period bound to unique payment.
func (s *Store) UpsertSubscriptionByPayment(ctx context.Context, sub domain.Subscription) error {
	now := time.Now().UTC()
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = now
	}
	if sub.UpdatedAt.IsZero() {
		sub.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO subscriptions (
			telegram_id, connector_id, payment_id, status, auto_pay_enabled, starts_at, ends_at, reminder_sent_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (payment_id)
		DO UPDATE SET
			status = EXCLUDED.status,
			auto_pay_enabled = EXCLUDED.auto_pay_enabled,
			starts_at = EXCLUDED.starts_at,
			ends_at = EXCLUDED.ends_at,
			reminder_sent_at = EXCLUDED.reminder_sent_at,
			updated_at = EXCLUDED.updated_at
	`,
		sub.TelegramID,
		sub.ConnectorID,
		sub.PaymentID,
		string(sub.Status),
		sub.AutoPayEnabled,
		sub.StartsAt,
		sub.EndsAt,
		sub.ReminderSentAt,
		sub.CreatedAt,
		sub.UpdatedAt,
	)
	return err
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
	if query.TelegramID > 0 {
		where = append(where, "telegram_id = "+addArg(query.TelegramID))
	}
	if query.ConnectorID > 0 {
		where = append(where, "connector_id = "+addArg(query.ConnectorID))
	}
	if query.Status != "" {
		where = append(where, "status = "+addArg(string(query.Status)))
	}
	if query.CreatedFrom != nil {
		where = append(where, "created_at >= "+addArg(*query.CreatedFrom))
	}
	if query.CreatedToExclude != nil {
		where = append(where, "created_at < "+addArg(*query.CreatedToExclude))
	}
	whereClause := strings.Join(where, " AND ")
	sqlText := fmt.Sprintf(`
		SELECT id, provider, provider_payment_id, status, token, telegram_id, connector_id, auto_pay_enabled,
		       amount_rub, checkout_url, created_at, paid_at, updated_at
		FROM payments
		WHERE %s
		ORDER BY created_at DESC, id DESC
		LIMIT %s
	`, whereClause, addArg(limit))

	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.Payment, 0, limit)
	for rows.Next() {
		var item domain.Payment
		var status string
		if err := rows.Scan(
			&item.ID,
			&item.Provider,
			&item.ProviderPaymentID,
			&status,
			&item.Token,
			&item.TelegramID,
			&item.ConnectorID,
			&item.AutoPayEnabled,
			&item.AmountRUB,
			&item.CheckoutURL,
			&item.CreatedAt,
			&item.PaidAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Status = domain.PaymentStatus(status)
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
	if query.TelegramID > 0 {
		where = append(where, "telegram_id = "+addArg(query.TelegramID))
	}
	if query.ConnectorID > 0 {
		where = append(where, "connector_id = "+addArg(query.ConnectorID))
	}
	if query.Status != "" {
		where = append(where, "status = "+addArg(string(query.Status)))
	}
	if query.CreatedFrom != nil {
		where = append(where, "created_at >= "+addArg(*query.CreatedFrom))
	}
	if query.CreatedToExclude != nil {
		where = append(where, "created_at < "+addArg(*query.CreatedToExclude))
	}
	whereClause := strings.Join(where, " AND ")
	sqlText := fmt.Sprintf(`
		SELECT id, telegram_id, connector_id, payment_id, status, auto_pay_enabled, starts_at, ends_at, reminder_sent_at, created_at, updated_at
		FROM subscriptions
		WHERE %s
		ORDER BY created_at DESC, id DESC
		LIMIT %s
	`, whereClause, addArg(limit))

	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.Subscription, 0, limit)
	for rows.Next() {
		var item domain.Subscription
		var status string
		if err := rows.Scan(
			&item.ID,
			&item.TelegramID,
			&item.ConnectorID,
			&item.PaymentID,
			&status,
			&item.AutoPayEnabled,
			&item.StartsAt,
			&item.EndsAt,
			&item.ReminderSentAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Status = domain.SubscriptionStatus(status)
		result = append(result, item)
	}
	return result, rows.Err()
}

// ListSubscriptionsForReminder returns active subscriptions that need 5-day reminder.
func (s *Store) ListSubscriptionsForReminder(ctx context.Context, remindBefore time.Time, limit int) ([]domain.Subscription, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	now := time.Now().UTC()
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, telegram_id, connector_id, payment_id, status, auto_pay_enabled, starts_at, ends_at, reminder_sent_at, created_at, updated_at
		FROM subscriptions
		WHERE status = $1
		  AND reminder_sent_at IS NULL
		  AND ends_at > $2
		  AND ends_at <= $3
		ORDER BY ends_at ASC
		LIMIT $4
	`, string(domain.SubscriptionStatusActive), now, remindBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Subscription, 0, limit)
	for rows.Next() {
		var item domain.Subscription
		var status string
		if err := rows.Scan(
			&item.ID,
			&item.TelegramID,
			&item.ConnectorID,
			&item.PaymentID,
			&status,
			&item.AutoPayEnabled,
			&item.StartsAt,
			&item.EndsAt,
			&item.ReminderSentAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Status = domain.SubscriptionStatus(status)
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
		SELECT id, telegram_id, connector_id, payment_id, status, auto_pay_enabled, starts_at, ends_at, reminder_sent_at, created_at, updated_at
		FROM subscriptions
		WHERE status = $1
		  AND ends_at <= $2
		ORDER BY ends_at ASC
		LIMIT $3
	`, string(domain.SubscriptionStatusActive), now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Subscription, 0, limit)
	for rows.Next() {
		var item domain.Subscription
		var status string
		if err := rows.Scan(
			&item.ID,
			&item.TelegramID,
			&item.ConnectorID,
			&item.PaymentID,
			&status,
			&item.AutoPayEnabled,
			&item.StartsAt,
			&item.EndsAt,
			&item.ReminderSentAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Status = domain.SubscriptionStatus(status)
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
