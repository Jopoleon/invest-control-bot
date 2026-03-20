-- +migrate Up
DROP INDEX IF EXISTS idx_legal_documents_active_per_type;

-- +migrate Down
CREATE UNIQUE INDEX IF NOT EXISTS idx_legal_documents_active_per_type
    ON legal_documents (doc_type)
    WHERE is_active = TRUE;
