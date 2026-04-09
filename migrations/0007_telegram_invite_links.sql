-- +migrate Up

CREATE TABLE IF NOT EXISTS telegram_invite_links (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    connector_id BIGINT NOT NULL REFERENCES connectors(id) ON DELETE CASCADE,
    subscription_id BIGINT NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    chat_ref TEXT NOT NULL,
    invite_link TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_telegram_invite_links_user_chat_active
    ON telegram_invite_links (user_id, chat_ref, created_at DESC)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_telegram_invite_links_subscription
    ON telegram_invite_links (subscription_id);

COMMENT ON TABLE telegram_invite_links IS 'Telegram invite links issued for paid access so they can be revoked when access ends.';
COMMENT ON COLUMN telegram_invite_links.chat_ref IS 'Resolved Telegram chat reference used to create and later revoke the invite link.';

-- +migrate Down

DROP TABLE IF EXISTS telegram_invite_links;
