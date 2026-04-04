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

func TestSaveConsent_UpsertsRow(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	acceptedAt := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO user_consents (
			user_id, connector_id, offer_accepted_at, privacy_accepted_at,
			offer_document_id, offer_document_version, privacy_document_id, privacy_document_version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (user_id, connector_id)
		DO UPDATE SET
			offer_accepted_at = EXCLUDED.offer_accepted_at,
			privacy_accepted_at = EXCLUDED.privacy_accepted_at,
			offer_document_id = EXCLUDED.offer_document_id,
			offer_document_version = EXCLUDED.offer_document_version,
			privacy_document_id = EXCLUDED.privacy_document_id,
			privacy_document_version = EXCLUDED.privacy_document_version
	`)).
		WithArgs(int64(17), int64(11), acceptedAt, acceptedAt, int64(1), 2, int64(2), 3).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.SaveConsent(context.Background(), domain.Consent{
		UserID:                 17,
		ConnectorID:            11,
		OfferAcceptedAt:        acceptedAt,
		PrivacyAcceptedAt:      acceptedAt,
		OfferDocumentID:        1,
		OfferDocumentVersion:   2,
		PrivacyDocumentID:      2,
		PrivacyDocumentVersion: 3,
	})
	if err != nil {
		t.Fatalf("SaveConsent: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestGetConsent_NotFound(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT user_id, connector_id, offer_accepted_at, privacy_accepted_at,
		       offer_document_id, offer_document_version, privacy_document_id, privacy_document_version
		FROM user_consents
		WHERE user_id = $1 AND connector_id = $2
	`)).
		WithArgs(int64(17), int64(11)).
		WillReturnError(sql.ErrNoRows)

	consent, found, err := store.GetConsent(context.Background(), 17, 11)
	if err != nil {
		t.Fatalf("GetConsent: %v", err)
	}
	if found {
		t.Fatalf("found = true, consent=%+v", consent)
	}
}

func TestListRecurringConsentsByUser_ReturnsHistory(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	acceptedAt := time.Date(2026, 4, 4, 9, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "connector_id", "accepted_at",
		"offer_document_id", "offer_document_version",
		"user_agreement_document_id", "user_agreement_document_version",
	}).AddRow(5, 17, 11, acceptedAt, 1, 2, 3, 4)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, user_id, connector_id, accepted_at,
		       offer_document_id, offer_document_version,
		       user_agreement_document_id, user_agreement_document_version
		FROM recurring_consents
		WHERE user_id = $1
		ORDER BY accepted_at DESC, id DESC
	`)).
		WithArgs(int64(17)).
		WillReturnRows(rows)

	items, err := store.ListRecurringConsentsByUser(context.Background(), 17)
	if err != nil {
		t.Fatalf("ListRecurringConsentsByUser: %v", err)
	}
	if len(items) != 1 || items[0].ID != 5 || items[0].UserAgreementDocumentID != 3 {
		t.Fatalf("unexpected items: %+v", items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
