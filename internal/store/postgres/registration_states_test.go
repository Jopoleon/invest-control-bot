package postgres

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func TestSaveRegistrationState_UpsertsRow(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO registration_states (
			messenger_kind, messenger_user_id, connector_id, step, username, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (messenger_kind, messenger_user_id)
		DO UPDATE SET
			connector_id = EXCLUDED.connector_id,
			step = EXCLUDED.step,
			username = EXCLUDED.username,
			updated_at = EXCLUDED.updated_at
	`)).
		WithArgs(string(domain.MessengerKindMAX), "193465776", int64(11), string(domain.StepEmail), "fedor", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.SaveRegistrationState(context.Background(), domain.RegistrationState{
		MessengerKind:   domain.MessengerKindMAX,
		MessengerUserID: "193465776",
		ConnectorID:     11,
		Step:            domain.StepEmail,
		Username:        "fedor",
	})
	if err != nil {
		t.Fatalf("SaveRegistrationState: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestGetRegistrationState(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	updatedAt := time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"messenger_kind", "messenger_user_id", "connector_id", "step", "username", "updated_at",
	}).AddRow("telegram", "264704572", 11, "username", "emiloserdov", updatedAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT messenger_kind, messenger_user_id, connector_id, step, username, updated_at
		FROM registration_states
		WHERE messenger_kind = $1 AND messenger_user_id = $2
	`)).
		WithArgs(string(domain.MessengerKindTelegram), "264704572").
		WillReturnRows(rows)

	state, found, err := store.GetRegistrationState(context.Background(), domain.MessengerKindTelegram, "264704572")
	if err != nil {
		t.Fatalf("GetRegistrationState: %v", err)
	}
	if !found {
		t.Fatal("expected registration state to be found")
	}
	if state.Step != domain.StepUsername || state.Username != "emiloserdov" {
		t.Fatalf("unexpected state: %+v", state)
	}
}

func TestGetRegistrationState_NotFoundAndDelete(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT messenger_kind, messenger_user_id, connector_id, step, username, updated_at
		FROM registration_states
		WHERE messenger_kind = $1 AND messenger_user_id = $2
	`)).
		WithArgs(string(domain.MessengerKindTelegram), "missing-user").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM registration_states WHERE messenger_kind = $1 AND messenger_user_id = $2`)).
		WithArgs(string(domain.MessengerKindTelegram), "missing-user").
		WillReturnResult(sqlmock.NewResult(0, 1))

	state, found, err := store.GetRegistrationState(context.Background(), domain.MessengerKindTelegram, "missing-user")
	if err != nil {
		t.Fatalf("GetRegistrationState: %v", err)
	}
	if found {
		t.Fatalf("expected no state, got %+v", state)
	}
	if err := store.DeleteRegistrationState(context.Background(), domain.MessengerKindTelegram, "missing-user"); err != nil {
		t.Fatalf("DeleteRegistrationState: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
