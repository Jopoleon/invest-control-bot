-- +migrate Up
CREATE TABLE IF NOT EXISTS recurring_consents (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    telegram_id BIGINT NOT NULL,
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    accepted_at TIMESTAMPTZ NOT NULL,
    offer_document_id BIGINT NOT NULL DEFAULT 0,
    offer_document_version INT NOT NULL DEFAULT 0,
    user_agreement_document_id BIGINT NOT NULL DEFAULT 0,
    user_agreement_document_version INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_recurring_consents_telegram_id
    ON recurring_consents (telegram_id, accepted_at DESC, id DESC);

-- +migrate Down
DROP TABLE IF EXISTS recurring_consents;
