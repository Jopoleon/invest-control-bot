package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	storepkg "github.com/Jopoleon/invest-control-bot/internal/store"
)

func TestGetConnector(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	createdAt := time.Date(2026, 3, 24, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "start_payload", "name", "description", "chat_id", "channel_url", "price_rub", "period_days",
		"test_period_seconds", "offer_url", "privacy_url", "is_active", "created_at",
	}).AddRow(11, "in-abc", "Recurring", "desc", "", "https://t.me/test", 2300, 30, 900, "http://offer", "http://policy", true, createdAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, start_payload, name, description, chat_id, channel_url, price_rub, period_days,
		       test_period_seconds, offer_url, privacy_url, is_active, created_at
		FROM connectors
		WHERE id = $1
	`)).WithArgs(int64(11)).WillReturnRows(rows)

	connector, found, err := store.GetConnector(context.Background(), 11)
	if err != nil {
		t.Fatalf("GetConnector: %v", err)
	}
	if !found {
		t.Fatal("expected connector to be found")
	}
	if connector.Name != "Recurring" || connector.StartPayload != "in-abc" || connector.PriceRUB != 2300 {
		t.Fatalf("unexpected connector: %+v", connector)
	}
	if connector.TestPeriodSeconds != 900 {
		t.Fatalf("unexpected test period seconds: %+v", connector)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestDeleteConnectorInUse(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS(SELECT 1 FROM connectors WHERE id = $1)`)).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS(SELECT 1 FROM payments WHERE connector_id = $1)`)).
		WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	err := store.DeleteConnector(context.Background(), 11)
	if err != storepkg.ErrConnectorInUse {
		t.Fatalf("expected ErrConnectorInUse, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
