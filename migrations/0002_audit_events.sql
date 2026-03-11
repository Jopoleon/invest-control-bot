-- +migrate Up
CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    telegram_id BIGINT NOT NULL DEFAULT 0,
    connector_id TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    details TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_events_created_at ON audit_events (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_telegram_id ON audit_events (telegram_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_connector_id ON audit_events (connector_id);

-- +migrate Down
DROP INDEX IF EXISTS idx_audit_events_connector_id;
DROP INDEX IF EXISTS idx_audit_events_telegram_id;
DROP INDEX IF EXISTS idx_audit_events_created_at;
DROP TABLE IF EXISTS audit_events;
