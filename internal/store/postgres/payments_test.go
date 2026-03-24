package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

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
