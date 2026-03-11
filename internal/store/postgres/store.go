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
	if c.ID == "" {
		return errors.New("connector ID is required")
	}
	if c.StartPayload == "" {
		c.StartPayload = c.ID
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO connectors (
			id, start_payload, name, description, chat_id, price_rub, period_days,
			offer_url, privacy_url, is_active, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`,
		c.ID,
		c.StartPayload,
		c.Name,
		c.Description,
		c.ChatID,
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
		SELECT id, start_payload, name, description, chat_id, price_rub, period_days,
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
func (s *Store) GetConnector(ctx context.Context, connectorID string) (domain.Connector, bool, error) {
	var c domain.Connector
	err := s.db.QueryRowContext(ctx, `
		SELECT id, start_payload, name, description, chat_id, price_rub, period_days,
		       offer_url, privacy_url, is_active, created_at
		FROM connectors
		WHERE id = $1
	`, connectorID).Scan(
		&c.ID,
		&c.StartPayload,
		&c.Name,
		&c.Description,
		&c.ChatID,
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
		SELECT id, start_payload, name, description, chat_id, price_rub, period_days,
		       offer_url, privacy_url, is_active, created_at
		FROM connectors
		WHERE start_payload = $1
	`, payload).Scan(
		&c.ID,
		&c.StartPayload,
		&c.Name,
		&c.Description,
		&c.ChatID,
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
func (s *Store) SetConnectorActive(ctx context.Context, connectorID string, active bool) error {
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
func (s *Store) GetConsent(ctx context.Context, telegramID int64, connectorID string) (domain.Consent, bool, error) {
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
	if query.ConnectorID != "" {
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
