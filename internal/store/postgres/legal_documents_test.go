package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func TestCreateLegalDocument_AssignsNextVersion(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`
			SELECT COALESCE(MAX(version), 0) + 1
			FROM legal_documents
			WHERE doc_type = $1
		`)).
		WithArgs(string(domain.LegalDocumentTypeOffer)).
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(4))
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO legal_documents (
			doc_type, title, content, external_url, version, is_active, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`)).
		WithArgs(string(domain.LegalDocumentTypeOffer), "Offer", "Body", "https://example.com/offer", 4, true, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectCommit()

	err := store.CreateLegalDocument(context.Background(), domain.LegalDocument{
		Type:        domain.LegalDocumentTypeOffer,
		Title:       "Offer",
		Content:     "Body",
		ExternalURL: "https://example.com/offer",
		IsActive:    true,
	})
	if err != nil {
		t.Fatalf("CreateLegalDocument: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestGetActiveLegalDocument_ReturnsLatest(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	createdAt := time.Date(2026, 4, 4, 9, 30, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "doc_type", "title", "content", "external_url", "version", "is_active", "created_at",
	}).AddRow(7, string(domain.LegalDocumentTypePrivacy), "Privacy", "Body", "https://example.com/privacy", 2, true, createdAt)

	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT id, doc_type, title, content, external_url, version, is_active, created_at
		FROM legal_documents
		WHERE doc_type = $1 AND is_active = TRUE
		ORDER BY version DESC, id DESC
		LIMIT 1
	`)).
		WithArgs(string(domain.LegalDocumentTypePrivacy)).
		WillReturnRows(rows)

	doc, found, err := store.GetActiveLegalDocument(context.Background(), domain.LegalDocumentTypePrivacy)
	if err != nil {
		t.Fatalf("GetActiveLegalDocument: %v", err)
	}
	if !found || doc.ID != 7 || doc.Version != 2 {
		t.Fatalf("unexpected doc: found=%v doc=%+v", found, doc)
	}
}

func TestSetAndDeleteLegalDocument(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	mock.ExpectExec(regexp.QuoteMeta(`
		UPDATE legal_documents
		SET is_active = $2
		WHERE id = $1
	`)).
		WithArgs(int64(7), false).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM legal_documents WHERE id = $1`)).
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.SetLegalDocumentActive(context.Background(), 7, false); err != nil {
		t.Fatalf("SetLegalDocumentActive: %v", err)
	}
	if err := store.DeleteLegalDocument(context.Background(), 7); err != nil {
		t.Fatalf("DeleteLegalDocument: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
