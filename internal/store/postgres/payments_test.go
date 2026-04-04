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

func TestCreatePaymentResolvesAndStoresUserID(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	now := time.Date(2026, 3, 26, 14, 0, 0, 0, time.UTC)
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
		INSERT INTO payments (
			provider, provider_payment_id, status, token, user_id, connector_id, subscription_id, parent_payment_id,
			auto_pay_enabled, amount_rub, checkout_url, created_at, paid_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`)).
		WithArgs(
			"robokassa",
			"",
			"pending",
			"inv-1",
			int64(17),
			int64(11),
			int64(0),
			int64(0),
			true,
			int64(2300),
			"https://example.com/pay",
			now,
			nil,
			now,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.CreatePayment(context.Background(), domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPending,
		Token:          "inv-1",
		UserID:         17,
		ConnectorID:    11,
		AmountRUB:      2300,
		AutoPayEnabled: true,
		CheckoutURL:    "https://example.com/pay",
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestUpdatePaymentPaidMarksRow(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	paidAt := time.Date(2026, 3, 24, 12, 30, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE payments
		SET status = $2, provider_payment_id = $3, paid_at = $4, updated_at = $5
		WHERE id = $1
		  AND status <> $2
	`)).
		WithArgs(int64(9), "paid", "robokassa:abc", paidAt, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	changed, err := store.UpdatePaymentPaid(context.Background(), 9, "robokassa:abc", paidAt)
	if err != nil {
		t.Fatalf("UpdatePaymentPaid: %v", err)
	}
	if !changed {
		t.Fatal("expected payment row to be updated")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestUpdatePaymentPaidReturnsNotFound(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	paidAt := time.Date(2026, 3, 24, 12, 30, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE payments
		SET status = $2, provider_payment_id = $3, paid_at = $4, updated_at = $5
		WHERE id = $1
		  AND status <> $2
	`)).
		WithArgs(int64(9), "paid", "robokassa:abc", paidAt, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM payments WHERE id = $1)`)).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	changed, err := store.UpdatePaymentPaid(context.Background(), 9, "robokassa:abc", paidAt)
	if err == nil || err.Error() != "payment not found" {
		t.Fatalf("expected payment not found error, got %v", err)
	}
	if changed {
		t.Fatal("expected changed=false for missing payment")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestGetPendingRebillPaymentBySubscriptionReturnsNewestPending(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	createdAt := time.Date(2026, 4, 4, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "provider", "provider_payment_id", "status", "token", "user_id", "connector_id",
		"subscription_id", "parent_payment_id", "auto_pay_enabled", "amount_rub", "checkout_url",
		"created_at", "paid_at", "updated_at",
	}).AddRow(
		44, "robokassa", "rebill_parent:parent-1", "pending", "child-44", 17, 11,
		29, 33, true, 2300, "", createdAt, nil, createdAt,
	)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT p.id, p.provider, p.provider_payment_id, p.status, p.token, COALESCE(p.user_id, 0),
		       p.connector_id, p.subscription_id, p.parent_payment_id,
		       p.auto_pay_enabled, p.amount_rub, p.checkout_url, p.created_at, p.paid_at, p.updated_at
		FROM payments p
		WHERE p.subscription_id = $1
		  AND p.status = $2
		ORDER BY p.created_at DESC, p.id DESC
		LIMIT 1
	`)).
		WithArgs(int64(29), string(domain.PaymentStatusPending)).
		WillReturnRows(rows)

	payment, found, err := store.GetPendingRebillPaymentBySubscription(context.Background(), 29)
	if err != nil {
		t.Fatalf("GetPendingRebillPaymentBySubscription: %v", err)
	}
	if !found {
		t.Fatal("expected pending payment to be found")
	}
	if payment.ID != 44 || payment.Token != "child-44" {
		t.Fatalf("payment=%+v want id=44 token=child-44", payment)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestUpdatePaymentFailedMarksPendingRow(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	updatedAt := time.Date(2026, 4, 4, 13, 45, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE payments
		SET status = $2, provider_payment_id = $3, updated_at = $4
		WHERE id = $1
		  AND status = $5
	`)).
		WithArgs(int64(14), "failed", "robokassa:child-14", updatedAt, "pending").
		WillReturnResult(sqlmock.NewResult(0, 1))

	changed, err := store.UpdatePaymentFailed(context.Background(), 14, "robokassa:child-14", updatedAt)
	if err != nil {
		t.Fatalf("UpdatePaymentFailed: %v", err)
	}
	if !changed {
		t.Fatal("expected payment row to be marked failed")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestUpdatePaymentFailedReturnsFalseForAlreadyPaidPayment(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	updatedAt := time.Date(2026, 4, 4, 13, 45, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE payments
		SET status = $2, provider_payment_id = $3, updated_at = $4
		WHERE id = $1
		  AND status = $5
	`)).
		WithArgs(int64(15), "failed", "robokassa:child-15", updatedAt, "pending").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM payments WHERE id = $1)`)).
		WithArgs(int64(15)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT status FROM payments WHERE id = $1`)).
		WithArgs(int64(15)).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("paid"))

	changed, err := store.UpdatePaymentFailed(context.Background(), 15, "robokassa:child-15", updatedAt)
	if err != nil {
		t.Fatalf("UpdatePaymentFailed: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false for already paid payment")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestUpdatePaymentPaidReturnsFalseForAlreadyPaidPayment(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	paidAt := time.Date(2026, 4, 4, 14, 30, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE payments
		SET status = $2, provider_payment_id = $3, paid_at = $4, updated_at = $5
		WHERE id = $1
		  AND status <> $2
	`)).
		WithArgs(int64(16), "paid", "robokassa:already-paid", paidAt, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS (SELECT 1 FROM payments WHERE id = $1)`)).
		WithArgs(int64(16)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	changed, err := store.UpdatePaymentPaid(context.Background(), 16, "robokassa:already-paid", paidAt)
	if err != nil {
		t.Fatalf("UpdatePaymentPaid: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false for already paid payment")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestGetPaymentByTokenReturnsNotFound(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT p.id, p.provider, p.provider_payment_id, p.status, p.token, COALESCE(p.user_id, 0),
		       p.connector_id, p.subscription_id, p.parent_payment_id,
		       p.auto_pay_enabled, p.amount_rub, p.checkout_url, p.created_at, p.paid_at, p.updated_at
		FROM payments p
		WHERE p.token = $1
	`)).
		WithArgs("missing-token").
		WillReturnError(sql.ErrNoRows)

	payment, found, err := store.GetPaymentByToken(context.Background(), "missing-token")
	if err != nil {
		t.Fatalf("GetPaymentByToken: %v", err)
	}
	if found {
		t.Fatalf("found=%v want=false payment=%+v", found, payment)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestGetPaymentByIDReturnsNotFound(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT p.id, p.provider, p.provider_payment_id, p.status, p.token, COALESCE(p.user_id, 0),
		       p.connector_id, p.subscription_id, p.parent_payment_id,
		       p.auto_pay_enabled, p.amount_rub, p.checkout_url, p.created_at, p.paid_at, p.updated_at
		FROM payments p
		WHERE p.id = $1
	`)).
		WithArgs(int64(404)).
		WillReturnError(sql.ErrNoRows)

	payment, found, err := store.GetPaymentByID(context.Background(), 404)
	if err != nil {
		t.Fatalf("GetPaymentByID: %v", err)
	}
	if found {
		t.Fatalf("found=%v want=false payment=%+v", found, payment)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
