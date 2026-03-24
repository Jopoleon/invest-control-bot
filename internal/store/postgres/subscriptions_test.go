package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestSetSubscriptionAutoPayEnabledNotFound(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	updatedAt := time.Date(2026, 3, 24, 13, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE subscriptions
		SET auto_pay_enabled = $2, updated_at = $3
		WHERE id = $1
	`)).
		WithArgs(int64(41), true, updatedAt).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := store.SetSubscriptionAutoPayEnabled(context.Background(), 41, true, updatedAt)
	if err == nil || err.Error() != "subscription not found" {
		t.Fatalf("expected subscription not found error, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestDisableAutoPayForActiveSubscriptions(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	updatedAt := time.Date(2026, 3, 24, 13, 15, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE subscriptions
		SET auto_pay_enabled = false, updated_at = $2
		WHERE telegram_id = $1
		  AND status = $3
		  AND auto_pay_enabled = true
	`)).
		WithArgs(int64(264704572), updatedAt, "active").
		WillReturnResult(sqlmock.NewResult(0, 2))

	if err := store.DisableAutoPayForActiveSubscriptions(context.Background(), 264704572, updatedAt); err != nil {
		t.Fatalf("DisableAutoPayForActiveSubscriptions: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
