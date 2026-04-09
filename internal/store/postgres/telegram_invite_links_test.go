package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func TestSaveTelegramInviteLink(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	expiresAt := time.Date(2026, 4, 9, 12, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, full_name, phone, email, created_at, updated_at
		FROM users
		WHERE id = $1
	`)).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "full_name", "phone", "email", "created_at", "updated_at"}).
			AddRow(int64(42), "", "", "", createdAt, createdAt))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT user_id, messenger_kind, messenger_user_id, username, linked_at, updated_at
		FROM user_messenger_accounts
		WHERE user_id = $1 AND messenger_kind = $2
	`)).
		WithArgs(int64(42), string(domain.MessengerKindTelegram)).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "messenger_kind", "messenger_user_id", "username", "linked_at", "updated_at",
		}))
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO telegram_invite_links (
			user_id, connector_id, subscription_id, chat_ref, invite_link, expires_at, revoked_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`)).
		WithArgs(int64(42), int64(11), int64(77), "@testchat", "https://t.me/+invite", &expiresAt, nil, createdAt).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.SaveTelegramInviteLink(context.Background(), domain.TelegramInviteLink{
		UserID:         42,
		ConnectorID:    11,
		SubscriptionID: 77,
		ChatRef:        "@testchat",
		InviteLink:     "https://t.me/+invite",
		ExpiresAt:      &expiresAt,
		CreatedAt:      createdAt,
	})
	if err != nil {
		t.Fatalf("SaveTelegramInviteLink: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListActiveTelegramInviteLinks(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	now := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "connector_id", "subscription_id", "chat_ref", "invite_link", "expires_at", "revoked_at", "created_at",
	}).AddRow(int64(1), int64(42), int64(11), int64(77), "@testchat", "https://t.me/+invite", expiresAt, nil, now)
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, user_id, connector_id, subscription_id, chat_ref, invite_link, expires_at, revoked_at, created_at
		FROM telegram_invite_links
		WHERE user_id = $1
		  AND chat_ref = $2
		  AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > $3)
		ORDER BY created_at DESC, id DESC
	`)).
		WithArgs(int64(42), "@testchat", now).
		WillReturnRows(rows)

	links, err := store.ListActiveTelegramInviteLinks(context.Background(), 42, "@testchat", now)
	if err != nil {
		t.Fatalf("ListActiveTelegramInviteLinks: %v", err)
	}
	if len(links) != 1 || links[0].InviteLink != "https://t.me/+invite" {
		t.Fatalf("links=%+v want 1 invite", links)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListRevocableTelegramInviteLinks(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	now := time.Date(2026, 4, 9, 10, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Minute)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "connector_id", "subscription_id", "chat_ref", "invite_link", "expires_at", "revoked_at", "created_at",
	}).AddRow(int64(1), int64(42), int64(11), int64(77), "@testchat", "https://t.me/+expired-but-not-revoked", expiredAt, nil, now)
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, user_id, connector_id, subscription_id, chat_ref, invite_link, expires_at, revoked_at, created_at
		FROM telegram_invite_links
		WHERE user_id = $1
		  AND chat_ref = $2
		  AND revoked_at IS NULL
		ORDER BY created_at DESC, id DESC
	`)).
		WithArgs(int64(42), "@testchat").
		WillReturnRows(rows)

	links, err := store.ListRevocableTelegramInviteLinks(context.Background(), 42, "@testchat")
	if err != nil {
		t.Fatalf("ListRevocableTelegramInviteLinks: %v", err)
	}
	if len(links) != 1 || links[0].InviteLink != "https://t.me/+expired-but-not-revoked" {
		t.Fatalf("links=%+v want expired-but-not-revoked invite", links)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestMarkTelegramInviteLinkRevoked(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	revokedAt := time.Date(2026, 4, 9, 10, 5, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE telegram_invite_links
		SET revoked_at = $2
		WHERE id = $1
		  AND revoked_at IS NULL
	`)).
		WithArgs(int64(7), revokedAt).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.MarkTelegramInviteLinkRevoked(context.Background(), 7, revokedAt); err != nil {
		t.Fatalf("MarkTelegramInviteLinkRevoked: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
