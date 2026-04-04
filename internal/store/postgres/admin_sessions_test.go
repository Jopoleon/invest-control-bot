package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func TestListAdminSessions_ReturnsRecentItems(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	now := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "session_token_hash", "subject", "created_at", "expires_at", "last_seen_at",
		"revoked_at", "ip", "user_agent", "rotated_at", "replaced_by_hash",
	}).AddRow(1, "hash", "egor", now.Add(-time.Hour), now.Add(time.Hour), now, nil, "127.0.0.1", "test-agent", nil, "")

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, session_token_hash, subject, created_at, expires_at, last_seen_at,
		       revoked_at, ip, user_agent, rotated_at, replaced_by_hash
		FROM admin_sessions
		ORDER BY created_at DESC, id DESC
		LIMIT $1
	`)).
		WithArgs(100).
		WillReturnRows(rows)

	items, err := store.ListAdminSessions(context.Background(), 0)
	if err != nil {
		t.Fatalf("ListAdminSessions: %v", err)
	}
	if len(items) != 1 || items[0].Subject != "egor" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestRotateAndCleanupAdminSessions(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	now := time.Date(2026, 4, 4, 10, 30, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE admin_sessions
		SET replaced_by_hash = $2,
		    session_token_hash = $2,
		    rotated_at = $3,
		    last_seen_at = $3
		WHERE id = $1
	`)).
		WithArgs(int64(1), "new-hash", now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
		DELETE FROM admin_sessions
		WHERE revoked_at IS NOT NULL OR expires_at < $1
	`)).
		WithArgs(now).
		WillReturnResult(sqlmock.NewResult(0, 2))

	if err := store.RotateAdminSession(context.Background(), 1, "new-hash", now); err != nil {
		t.Fatalf("RotateAdminSession: %v", err)
	}
	if err := store.CleanupAdminSessions(context.Background(), now); err != nil {
		t.Fatalf("CleanupAdminSessions: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestCreateAdminSession(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	now := time.Date(2026, 4, 4, 11, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO admin_sessions (
			session_token_hash, subject, created_at, expires_at, last_seen_at,
			revoked_at, ip, user_agent, rotated_at, replaced_by_hash
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`)).
		WithArgs("hash", "egor", now, now.Add(time.Hour), now, nil, "127.0.0.1", "test-agent", nil, "").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.CreateAdminSession(context.Background(), domain.AdminSession{
		TokenHash:  "hash",
		Subject:    "egor",
		CreatedAt:  now,
		ExpiresAt:  now.Add(time.Hour),
		LastSeenAt: now,
		IP:         "127.0.0.1",
		UserAgent:  "test-agent",
	})
	if err != nil {
		t.Fatalf("CreateAdminSession: %v", err)
	}
}
