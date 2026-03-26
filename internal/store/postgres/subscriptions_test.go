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

func TestUpsertSubscriptionByPaymentResolvesAndStoresUserID(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	now := time.Date(2026, 3, 26, 14, 30, 0, 0, time.UTC)
	startsAt := now
	endsAt := now.Add(30 * 24 * time.Hour)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, COALESCE(telegram_id, 0), telegram_username, full_name, phone, email, updated_at
		FROM users
		WHERE telegram_id = $1
	`)).
		WithArgs(int64(264704572)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "telegram_id", "telegram_username", "full_name", "phone", "email", "updated_at",
		}).AddRow(17, 264704572, "emiloserdov", "Egor", "", "", now))

	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO subscriptions (
			user_id, telegram_id, connector_id, payment_id, status, auto_pay_enabled, starts_at, ends_at, reminder_sent_at, expiry_notice_sent_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (payment_id)
		DO UPDATE SET
			user_id = EXCLUDED.user_id,
			status = EXCLUDED.status,
			auto_pay_enabled = EXCLUDED.auto_pay_enabled,
			starts_at = EXCLUDED.starts_at,
			ends_at = EXCLUDED.ends_at,
			reminder_sent_at = EXCLUDED.reminder_sent_at,
			expiry_notice_sent_at = EXCLUDED.expiry_notice_sent_at,
			updated_at = EXCLUDED.updated_at
	`)).
		WithArgs(
			sql.NullInt64{Int64: 17, Valid: true},
			int64(264704572),
			int64(11),
			int64(9),
			"active",
			true,
			startsAt,
			endsAt,
			nil,
			nil,
			now,
			now,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.UpsertSubscriptionByPayment(context.Background(), domain.Subscription{
		TelegramID:     264704572,
		ConnectorID:    11,
		PaymentID:      9,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       startsAt,
		EndsAt:         endsAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("UpsertSubscriptionByPayment: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

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
