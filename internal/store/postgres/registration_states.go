package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

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
