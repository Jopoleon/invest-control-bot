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
			messenger_kind, messenger_user_id, connector_id, step, username, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (messenger_kind, messenger_user_id)
		DO UPDATE SET
			connector_id = EXCLUDED.connector_id,
			step = EXCLUDED.step,
			username = EXCLUDED.username,
			updated_at = EXCLUDED.updated_at
	`, string(state.MessengerKind), state.MessengerUserID, state.ConnectorID, string(state.Step), state.Username, now)
	return err
}

// GetRegistrationState fetches registration FSM state.
func (s *Store) GetRegistrationState(ctx context.Context, kind domain.MessengerKind, messengerUserID string) (domain.RegistrationState, bool, error) {
	var state domain.RegistrationState
	var (
		step          string
		messengerKind string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT messenger_kind, messenger_user_id, connector_id, step, username, updated_at
		FROM registration_states
		WHERE messenger_kind = $1 AND messenger_user_id = $2
	`, string(kind), messengerUserID).Scan(&messengerKind, &state.MessengerUserID, &state.ConnectorID, &step, &state.Username, &state.UpdatedAt)
	if err == sql.ErrNoRows {
		return domain.RegistrationState{}, false, nil
	}
	if err != nil {
		return domain.RegistrationState{}, false, err
	}
	state.MessengerKind = domain.MessengerKind(messengerKind)
	state.Step = domain.RegistrationStep(step)
	return state, true, nil
}

// DeleteRegistrationState removes registration FSM state.
func (s *Store) DeleteRegistrationState(ctx context.Context, kind domain.MessengerKind, messengerUserID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM registration_states WHERE messenger_kind = $1 AND messenger_user_id = $2`, string(kind), messengerUserID)
	return err
}
