package postgres

import (
	"context"
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
		SELECT id, full_name, phone, email, created_at, updated_at
		FROM users
		WHERE id = $1
	`)).
		WithArgs(int64(17)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "full_name", "phone", "email", "created_at", "updated_at",
		}).AddRow(17, "Egor", "", "", now.Add(-time.Hour), now))
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT user_id, messenger_kind, messenger_user_id, username, linked_at, updated_at
		FROM user_messenger_accounts
		WHERE user_id = $1 AND messenger_kind = $2
	`)).
		WithArgs(int64(17), string(domain.MessengerKindTelegram)).
		WillReturnRows(sqlmock.NewRows([]string{
			"user_id", "messenger_kind", "messenger_user_id", "username", "linked_at", "updated_at",
		}))

	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO subscriptions (
			user_id, connector_id, payment_id, status, auto_pay_enabled, starts_at, ends_at, reminder_sent_at, expiry_notice_sent_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
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
			int64(17),
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
		UserID:         17,
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
		WHERE user_id = $1
		  AND status = $3
		  AND auto_pay_enabled = true
	`)).
		WithArgs(int64(17), updatedAt, "active").
		WillReturnResult(sqlmock.NewResult(0, 2))

	if err := store.DisableAutoPayForActiveSubscriptions(context.Background(), 17, updatedAt); err != nil {
		t.Fatalf("DisableAutoPayForActiveSubscriptions: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestGetLatestSubscriptionByUserConnectorOrdersByEndsAtDescThenIDDesc(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	now := time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "connector_id", "payment_id", "status", "auto_pay_enabled",
		"starts_at", "ends_at", "reminder_sent_at", "expiry_notice_sent_at", "created_at", "updated_at",
	}).AddRow(
		99, 17, 11, 19, "active", true,
		now.Add(-24*time.Hour), now.Add(24*time.Hour), nil, nil, now.Add(-24*time.Hour), now,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT s.id, COALESCE(s.user_id, 0),
		       s.connector_id, s.payment_id, s.status, s.auto_pay_enabled, s.starts_at, s.ends_at,
		       s.reminder_sent_at, s.expiry_notice_sent_at, s.created_at, s.updated_at
		FROM subscriptions s
		WHERE s.user_id = $1 AND s.connector_id = $2
		ORDER BY s.ends_at DESC, s.id DESC
		LIMIT 1
	`)).
		WithArgs(int64(17), int64(11)).
		WillReturnRows(rows)

	sub, found, err := store.GetLatestSubscriptionByUserConnector(context.Background(), 17, 11)
	if err != nil {
		t.Fatalf("GetLatestSubscriptionByUserConnector: %v", err)
	}
	if !found {
		t.Fatal("expected subscription to be found")
	}
	if sub.ID != 99 {
		t.Fatalf("subscription id=%d want=99", sub.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListSubscriptionsForReminderExcludesFutureQueuedSubscriptions(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	remindBefore := time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "connector_id", "payment_id", "status", "auto_pay_enabled",
		"starts_at", "ends_at", "reminder_sent_at", "expiry_notice_sent_at", "created_at", "updated_at",
	}).AddRow(
		71, 17, 11, 19, "active", true,
		remindBefore.Add(-12*time.Hour), remindBefore.Add(-time.Hour), nil, nil, remindBefore.Add(-24*time.Hour), remindBefore.Add(-2*time.Hour),
	)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT s.id, COALESCE(s.user_id, 0),
		       s.connector_id, s.payment_id, s.status, s.auto_pay_enabled, s.starts_at, s.ends_at,
		       s.reminder_sent_at, s.expiry_notice_sent_at, s.created_at, s.updated_at
		FROM subscriptions s
		WHERE s.status = $1
		  AND s.starts_at <= $2
		  AND s.reminder_sent_at IS NULL
		  AND s.ends_at > $2
		  AND s.ends_at <= $3
		ORDER BY s.ends_at ASC
		LIMIT $4
	`)).
		WithArgs(string(domain.SubscriptionStatusActive), sqlmock.AnyArg(), remindBefore, 50).
		WillReturnRows(rows)

	subs, err := store.ListSubscriptionsForReminder(context.Background(), remindBefore, 50)
	if err != nil {
		t.Fatalf("ListSubscriptionsForReminder: %v", err)
	}
	if len(subs) != 1 || subs[0].ID != 71 {
		t.Fatalf("subscriptions=%+v want one row with id=71", subs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListSubscriptionsForExpiryNoticeExcludesFutureQueuedSubscriptions(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	noticeBefore := time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "connector_id", "payment_id", "status", "auto_pay_enabled",
		"starts_at", "ends_at", "reminder_sent_at", "expiry_notice_sent_at", "created_at", "updated_at",
	}).AddRow(
		81, 17, 11, 29, "active", true,
		noticeBefore.Add(-6*time.Hour), noticeBefore.Add(-30*time.Minute), nil, nil, noticeBefore.Add(-24*time.Hour), noticeBefore.Add(-2*time.Hour),
	)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT s.id, COALESCE(s.user_id, 0),
		       s.connector_id, s.payment_id, s.status, s.auto_pay_enabled, s.starts_at, s.ends_at,
		       s.reminder_sent_at, s.expiry_notice_sent_at, s.created_at, s.updated_at
		FROM subscriptions s
		WHERE s.status = $1
		  AND s.starts_at <= $2
		  AND s.expiry_notice_sent_at IS NULL
		  AND s.ends_at > $2
		  AND s.ends_at <= $3
		ORDER BY s.ends_at ASC
		LIMIT $4
	`)).
		WithArgs(string(domain.SubscriptionStatusActive), sqlmock.AnyArg(), noticeBefore, 25).
		WillReturnRows(rows)

	subs, err := store.ListSubscriptionsForExpiryNotice(context.Background(), noticeBefore, 25)
	if err != nil {
		t.Fatalf("ListSubscriptionsForExpiryNotice: %v", err)
	}
	if len(subs) != 1 || subs[0].ID != 81 {
		t.Fatalf("subscriptions=%+v want one row with id=81", subs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
