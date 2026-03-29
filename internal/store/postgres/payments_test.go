package postgres

import (
	"context"
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
		SELECT u.id, u.full_name, u.phone, u.email, u.created_at, u.updated_at
		FROM user_messenger_accounts a
		JOIN users u ON u.id = a.user_id
		WHERE a.messenger_kind = $1 AND a.messenger_user_id = $2
	`)).
		WithArgs(string(domain.MessengerKindTelegram), "264704572").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "full_name", "phone", "email", "created_at", "updated_at",
		}).AddRow(17, "Egor", "", "", now.Add(-time.Hour), now))

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
		TelegramID:     264704572,
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
