-- +migrate Up
CREATE TABLE IF NOT EXISTS admin_sessions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_token_hash TEXT NOT NULL UNIQUE,
    subject TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    rotated_at TIMESTAMPTZ NULL,
    replaced_by_hash TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_admin_sessions_expires_at ON admin_sessions (expires_at);
CREATE INDEX IF NOT EXISTS idx_admin_sessions_last_seen_at ON admin_sessions (last_seen_at);
CREATE INDEX IF NOT EXISTS idx_admin_sessions_revoked_at ON admin_sessions (revoked_at);

-- +migrate Down
DROP INDEX IF EXISTS idx_admin_sessions_revoked_at;
DROP INDEX IF EXISTS idx_admin_sessions_last_seen_at;
DROP INDEX IF EXISTS idx_admin_sessions_expires_at;
DROP TABLE IF EXISTS admin_sessions;
