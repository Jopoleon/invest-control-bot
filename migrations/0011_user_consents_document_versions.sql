-- +migrate Up
ALTER TABLE user_consents
    ADD COLUMN IF NOT EXISTS offer_document_id BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS offer_document_version INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS privacy_document_id BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS privacy_document_version INT NOT NULL DEFAULT 0;

-- +migrate Down
ALTER TABLE user_consents
    DROP COLUMN IF EXISTS privacy_document_version,
    DROP COLUMN IF EXISTS privacy_document_id,
    DROP COLUMN IF EXISTS offer_document_version,
    DROP COLUMN IF EXISTS offer_document_id;
