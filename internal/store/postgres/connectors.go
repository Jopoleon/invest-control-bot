package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	storepkg "github.com/Jopoleon/invest-control-bot/internal/store"
)

func (s *Store) CreateConnector(ctx context.Context, c domain.Connector) error {
	if c.StartPayload == "" {
		return errors.New("start payload is required")
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO connectors (
			start_payload, name, description, chat_id, channel_url, max_chat_id, max_channel_url, price_rub,
			period_mode, period_seconds, period_months, fixed_ends_at,
			offer_url, privacy_url, is_active, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	`,
		c.StartPayload,
		c.Name,
		c.Description,
		c.ChatID,
		c.ChannelURL,
		c.MAXChatID,
		c.MAXChannelURL,
		c.PriceRUB,
		c.PeriodMode,
		c.PeriodSeconds,
		c.PeriodMonths,
		c.FixedEndsAt,
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
		SELECT id, start_payload, name, description, chat_id, channel_url, max_chat_id, max_channel_url, price_rub,
		       period_mode, period_seconds, period_months, fixed_ends_at,
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
		var fixedEndsAt sql.NullTime
		if err := rows.Scan(
			&c.ID,
			&c.StartPayload,
			&c.Name,
			&c.Description,
			&c.ChatID,
			&c.ChannelURL,
			&c.MAXChatID,
			&c.MAXChannelURL,
			&c.PriceRUB,
			&c.PeriodMode,
			&c.PeriodSeconds,
			&c.PeriodMonths,
			&fixedEndsAt,
			&c.OfferURL,
			&c.PrivacyURL,
			&c.IsActive,
			&c.CreatedAt,
		); err != nil {
			return nil, err
		}
		if fixedEndsAt.Valid {
			value := fixedEndsAt.Time.UTC()
			c.FixedEndsAt = &value
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// GetConnector fetches connector by ID.
func (s *Store) GetConnector(ctx context.Context, connectorID int64) (domain.Connector, bool, error) {
	var c domain.Connector
	var fixedEndsAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, start_payload, name, description, chat_id, channel_url, max_chat_id, max_channel_url, price_rub,
		       period_mode, period_seconds, period_months, fixed_ends_at,
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
		&c.MAXChatID,
		&c.MAXChannelURL,
		&c.PriceRUB,
		&c.PeriodMode,
		&c.PeriodSeconds,
		&c.PeriodMonths,
		&fixedEndsAt,
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
	if fixedEndsAt.Valid {
		value := fixedEndsAt.Time.UTC()
		c.FixedEndsAt = &value
	}
	return c, true, nil
}

// GetConnectorByStartPayload fetches connector by deeplink payload.
func (s *Store) GetConnectorByStartPayload(ctx context.Context, payload string) (domain.Connector, bool, error) {
	var c domain.Connector
	var fixedEndsAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, start_payload, name, description, chat_id, channel_url, max_chat_id, max_channel_url, price_rub,
		       period_mode, period_seconds, period_months, fixed_ends_at,
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
		&c.MAXChatID,
		&c.MAXChannelURL,
		&c.PriceRUB,
		&c.PeriodMode,
		&c.PeriodSeconds,
		&c.PeriodMonths,
		&fixedEndsAt,
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
	if fixedEndsAt.Valid {
		value := fixedEndsAt.Time.UTC()
		c.FixedEndsAt = &value
	}
	return c, true, nil
}

// UpdateConnectorText changes only operator-facing connector copy. Commercial
// terms, start payload, and access destinations stay immutable through this
// path because active payments/subscriptions resolve them from the connector.
func (s *Store) UpdateConnectorText(ctx context.Context, connectorID int64, name, description string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE connectors
		SET name = $2, description = $3
		WHERE id = $1
	`, connectorID, name, description)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return storepkg.ErrConnectorNotFound
	}
	return nil
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
		return storepkg.ErrConnectorNotFound
	}
	return nil
}

// DeleteConnector hard-deletes connector only when it has no dependent business history.
func (s *Store) DeleteConnector(ctx context.Context, connectorID int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM connectors WHERE id = $1)`, connectorID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return storepkg.ErrConnectorNotFound
	}

	queries := []string{
		`SELECT EXISTS(SELECT 1 FROM payments WHERE connector_id = $1)`,
		`SELECT EXISTS(SELECT 1 FROM subscriptions WHERE connector_id = $1)`,
		`SELECT EXISTS(SELECT 1 FROM user_consents WHERE connector_id = $1)`,
		`SELECT EXISTS(SELECT 1 FROM registration_states WHERE connector_id = $1)`,
	}
	for _, query := range queries {
		var inUse bool
		if err := tx.QueryRowContext(ctx, query, connectorID).Scan(&inUse); err != nil {
			return err
		}
		if inUse {
			return storepkg.ErrConnectorInUse
		}
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM connectors WHERE id = $1`, connectorID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return storepkg.ErrConnectorNotFound
	}
	return tx.Commit()
}
