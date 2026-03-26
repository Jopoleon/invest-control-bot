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

func TestGetUserByID(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	updatedAt := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "telegram_id", "telegram_username", "full_name", "phone", "email", "updated_at",
	}).AddRow(7, 264704572, "emiloserdov", "Egor", "+79990001122", "egor@example.com", updatedAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, COALESCE(telegram_id, 0), telegram_username, full_name, phone, email, updated_at
		FROM users
		WHERE id = $1
	`)).WithArgs(int64(7)).WillReturnRows(rows)

	user, found, err := store.GetUserByID(context.Background(), 7)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if !found {
		t.Fatal("expected user to be found")
	}
	if user.ID != 7 || user.TelegramID != 264704572 {
		t.Fatalf("unexpected user: %+v", user)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestGetOrCreateUserByMessengerCreatesNewTelegramUser(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	updatedAt := time.Date(2026, 3, 26, 10, 30, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT u.id, COALESCE(u.telegram_id, 0), u.telegram_username, u.full_name, u.phone, u.email, u.updated_at
		FROM user_messenger_accounts a
		JOIN users u ON u.id = a.user_id
		WHERE a.messenger_kind = $1 AND a.external_user_id = $2
	`)).
		WithArgs(string(domain.MessengerKindTelegram), "264704572").
		WillReturnError(sql.ErrNoRows)

	mock.ExpectQuery(regexp.QuoteMeta(`
			SELECT id, COALESCE(telegram_id, 0), telegram_username, full_name, phone, email, updated_at
			FROM users
			WHERE telegram_id = $1
		`)).
		WithArgs(int64(264704572)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "telegram_id", "telegram_username", "full_name", "phone", "email", "updated_at"}))

	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO users (telegram_id, telegram_username, full_name, phone, email, updated_at)
		VALUES ($1,$2,'','','',$3)
		RETURNING id, COALESCE(telegram_id, 0), telegram_username, full_name, phone, email, updated_at
	`)).
		WithArgs(sqlmock.AnyArg(), "emiloserdov", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "telegram_id", "telegram_username", "full_name", "phone", "email", "updated_at",
		}).AddRow(11, 264704572, "emiloserdov", "", "", "", updatedAt))

	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO user_messenger_accounts (
			user_id, messenger_kind, external_user_id, username, linked_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6)
	`)).
		WithArgs(int64(11), string(domain.MessengerKindTelegram), "264704572", "emiloserdov", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	user, created, err := store.GetOrCreateUserByMessenger(context.Background(), domain.MessengerKindTelegram, "264704572", "emiloserdov")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	if !created {
		t.Fatalf("created = false, want true")
	}
	if user.ID != 11 || user.TelegramID != 264704572 {
		t.Fatalf("unexpected user: %+v", user)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListUserMessengerAccounts(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	linkedAt := time.Date(2026, 3, 26, 9, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 3, 26, 11, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"user_id", "messenger_kind", "external_user_id", "username", "linked_at", "updated_at",
	}).AddRow(11, "telegram", "264704572", "emiloserdov", linkedAt, updatedAt).
		AddRow(11, "max", "max-user-1", "egor.max", linkedAt, updatedAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT user_id, messenger_kind, external_user_id, username, linked_at, updated_at
		FROM user_messenger_accounts
		WHERE user_id = $1
		ORDER BY messenger_kind ASC, external_user_id ASC
	`)).WithArgs(int64(11)).WillReturnRows(rows)

	accounts, err := store.ListUserMessengerAccounts(context.Background(), 11)
	if err != nil {
		t.Fatalf("ListUserMessengerAccounts: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("accounts len = %d, want 2", len(accounts))
	}
	var (
		foundTelegram bool
		foundMAX      bool
	)
	for _, account := range accounts {
		switch account.MessengerKind {
		case domain.MessengerKindTelegram:
			if account.ExternalUserID == "264704572" {
				foundTelegram = true
			}
		case domain.MessengerKindMAX:
			if account.ExternalUserID == "max-user-1" {
				foundMAX = true
			}
		}
	}
	if !foundTelegram || !foundMAX {
		t.Fatalf("unexpected accounts content: %+v", accounts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
