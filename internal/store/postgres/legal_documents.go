package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (s *Store) CreateLegalDocument(ctx context.Context, doc domain.LegalDocument) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now().UTC()
	}
	if doc.Version <= 0 {
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(MAX(version), 0) + 1
			FROM legal_documents
			WHERE doc_type = $1
		`, string(doc.Type)).Scan(&doc.Version); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO legal_documents (
			doc_type, title, content, external_url, version, is_active, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`,
		string(doc.Type),
		doc.Title,
		doc.Content,
		doc.ExternalURL,
		doc.Version,
		doc.IsActive,
		doc.CreatedAt,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateLegalDocument updates existing legal document fields and active flag.
func (s *Store) UpdateLegalDocument(ctx context.Context, doc domain.LegalDocument) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var docType string
	if err := tx.QueryRowContext(ctx, `
		SELECT doc_type
		FROM legal_documents
		WHERE id = $1
	`, doc.ID).Scan(&docType); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE legal_documents
		SET title = $2,
		    content = $3,
		    external_url = $4,
		    is_active = $5
		WHERE id = $1
	`, doc.ID, doc.Title, doc.Content, doc.ExternalURL, doc.IsActive); err != nil {
		return err
	}
	return tx.Commit()
}

// ListLegalDocuments returns all legal document versions.
func (s *Store) ListLegalDocuments(ctx context.Context, docType domain.LegalDocumentType) ([]domain.LegalDocument, error) {
	query := `
		SELECT id, doc_type, title, content, external_url, version, is_active, created_at
		FROM legal_documents
	`
	args := []any{}
	if docType != "" {
		query += ` WHERE doc_type = $1`
		args = append(args, string(docType))
	}
	query += ` ORDER BY doc_type ASC, version DESC, created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.LegalDocument, 0)
	for rows.Next() {
		item, err := scanLegalDocument(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetLegalDocument fetches legal document by ID.
func (s *Store) GetLegalDocument(ctx context.Context, documentID int64) (domain.LegalDocument, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, doc_type, title, content, external_url, version, is_active, created_at
		FROM legal_documents
		WHERE id = $1
	`, documentID)
	item, err := scanLegalDocument(row)
	if err == sql.ErrNoRows {
		return domain.LegalDocument{}, false, nil
	}
	if err != nil {
		return domain.LegalDocument{}, false, err
	}
	return item, true, nil
}

// GetActiveLegalDocument fetches active document version by type.
func (s *Store) GetActiveLegalDocument(ctx context.Context, docType domain.LegalDocumentType) (domain.LegalDocument, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, doc_type, title, content, external_url, version, is_active, created_at
		FROM legal_documents
		WHERE doc_type = $1 AND is_active = TRUE
		ORDER BY version DESC, id DESC
		LIMIT 1
	`, string(docType))
	item, err := scanLegalDocument(row)
	if err == sql.ErrNoRows {
		return domain.LegalDocument{}, false, nil
	}
	if err != nil {
		return domain.LegalDocument{}, false, err
	}
	return item, true, nil
}

// SetLegalDocumentActive toggles published state for a single legal document.
func (s *Store) SetLegalDocumentActive(ctx context.Context, documentID int64, active bool) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE legal_documents
		SET is_active = $2
		WHERE id = $1
	`, documentID, active)
	return err
}

// DeleteLegalDocument removes legal document by ID.
func (s *Store) DeleteLegalDocument(ctx context.Context, documentID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM legal_documents WHERE id = $1`, documentID)
	return err
}
