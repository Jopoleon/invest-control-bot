-- +migrate Up
CREATE TABLE IF NOT EXISTS legal_documents (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    doc_type TEXT NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    external_url TEXT NOT NULL DEFAULT '',
    version INT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_legal_documents_type_version
    ON legal_documents (doc_type, version);

CREATE UNIQUE INDEX IF NOT EXISTS idx_legal_documents_active_per_type
    ON legal_documents (doc_type)
    WHERE is_active = TRUE;

-- +migrate Down
DROP INDEX IF EXISTS idx_legal_documents_active_per_type;
DROP INDEX IF EXISTS idx_legal_documents_type_version;
DROP TABLE IF EXISTS legal_documents;
