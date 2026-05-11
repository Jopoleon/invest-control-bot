-- +migrate Up

CREATE TABLE IF NOT EXISTS messenger_chat_users (
    messenger_kind TEXT NOT NULL,
    chat_ref TEXT NOT NULL,
    messenger_user_id TEXT NOT NULL,
    user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    status TEXT NOT NULL,
    is_member BOOLEAN NOT NULL DEFAULT FALSE,
    is_banned BOOLEAN NOT NULL DEFAULT FALSE,
    is_restricted BOOLEAN NOT NULL DEFAULT FALSE,
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    is_owner BOOLEAN NOT NULL DEFAULT FALSE,
    permissions JSONB NOT NULL DEFAULT '[]'::jsonb,
    checked_at TIMESTAMPTZ NOT NULL,
    last_joined_at TIMESTAMPTZ NULL,
    last_left_at TIMESTAMPTZ NULL,
    last_kicked_at TIMESTAMPTZ NULL,
    last_unbanned_at TIMESTAMPTZ NULL,
    last_error TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (messenger_kind, chat_ref, messenger_user_id)
);

CREATE INDEX IF NOT EXISTS idx_messenger_chat_users_user
    ON messenger_chat_users (user_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_messenger_chat_users_status
    ON messenger_chat_users (messenger_kind, status, checked_at DESC);

CREATE TABLE IF NOT EXISTS messenger_chat_user_checks (
    id BIGSERIAL PRIMARY KEY,
    messenger_kind TEXT NOT NULL,
    chat_ref TEXT NOT NULL,
    messenger_user_id TEXT NOT NULL,
    user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    connector_id BIGINT NULL REFERENCES connectors(id) ON DELETE SET NULL,
    subscription_id BIGINT NULL REFERENCES subscriptions(id) ON DELETE SET NULL,
    source TEXT NOT NULL,
    status TEXT NOT NULL,
    ok BOOLEAN NOT NULL,
    raw_json JSONB NULL,
    raw_error TEXT NOT NULL DEFAULT '',
    checked_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_messenger_chat_user_checks_target
    ON messenger_chat_user_checks (messenger_kind, chat_ref, messenger_user_id, checked_at DESC);

CREATE INDEX IF NOT EXISTS idx_messenger_chat_user_checks_user
    ON messenger_chat_user_checks (user_id, checked_at DESC);

COMMENT ON TABLE messenger_chat_users IS 'Latest normalized user membership status per messenger chat, used for admin diagnostics.';
COMMENT ON TABLE messenger_chat_user_checks IS 'Immutable history of chat membership checks and raw provider responses/errors.';

-- +migrate Down

DROP TABLE IF EXISTS messenger_chat_user_checks;
DROP TABLE IF EXISTS messenger_chat_users;
