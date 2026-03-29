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
		"id", "full_name", "phone", "email", "created_at", "updated_at",
	}).AddRow(7, "Egor", "+79990001122", "egor@example.com", updatedAt.Add(-time.Hour), updatedAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, full_name, phone, email, created_at, updated_at
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
	if user.ID != 7 || user.Email != "egor@example.com" {
		t.Fatalf("unexpected user: %+v", user)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestGetUserByMessenger(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()
	rows := sqlmock.NewRows([]string{"id", "full_name", "phone", "email", "created_at", "updated_at"}).
		AddRow(int64(9), "Egor", "", "", time.Unix(1709996400, 0).UTC(), time.Unix(1710000000, 0).UTC())

	mock.ExpectQuery(`SELECT u\.id, u\.full_name, u\.phone, u\.email, u\.created_at, u\.updated_at`).
		WithArgs(string(domain.MessengerKindTelegram), "555").
		WillReturnRows(rows)

	user, found, err := store.GetUserByMessenger(context.Background(), domain.MessengerKindTelegram, "555")
	if err != nil {
		t.Fatalf("GetUserByMessenger: %v", err)
	}
	if !found || user.ID != 9 || user.FullName != "Egor" {
		t.Fatalf("GetUserByMessenger returned %+v, found=%v", user, found)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestGetOrCreateUserByMessengerCreatesNewTelegramUser(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	updatedAt := time.Date(2026, 3, 26, 10, 30, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT u.id, u.full_name, u.phone, u.email, u.created_at, u.updated_at
		FROM user_messenger_accounts a
		JOIN users u ON u.id = a.user_id
		WHERE a.messenger_kind = $1 AND a.messenger_user_id = $2
	`)).
		WithArgs(string(domain.MessengerKindTelegram), "264704572").
		WillReturnError(sql.ErrNoRows)

	mock.ExpectQuery(regexp.QuoteMeta(`
		INSERT INTO users (full_name, phone, email, updated_at)
		VALUES ('','','',$1)
		RETURNING id, full_name, phone, email, created_at, updated_at
	`)).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "full_name", "phone", "email", "created_at", "updated_at",
		}).AddRow(11, "", "", "", updatedAt.Add(-time.Minute), updatedAt))

	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO user_messenger_accounts (
			user_id, messenger_kind, messenger_user_id, username, linked_at, updated_at
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
	if user.ID != 11 {
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
		"user_id", "messenger_kind", "messenger_user_id", "username", "linked_at", "updated_at",
	}).AddRow(11, "telegram", "264704572", "emiloserdov", linkedAt, updatedAt).
		AddRow(11, "max", "max-user-1", "egor.max", linkedAt, updatedAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT user_id, messenger_kind, messenger_user_id, username, linked_at, updated_at
		FROM user_messenger_accounts
		WHERE user_id = $1
		ORDER BY messenger_kind ASC, messenger_user_id ASC
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
			if account.MessengerUserID == "264704572" {
				foundTelegram = true
			}
		case domain.MessengerKindMAX:
			if account.MessengerUserID == "max-user-1" {
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
