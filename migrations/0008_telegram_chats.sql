-- +migrate Up

CREATE TABLE IF NOT EXISTS telegram_chats (
    chat_id TEXT PRIMARY KEY,
    title TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    chat_type TEXT NOT NULL DEFAULT '',
    bot_status TEXT NOT NULL DEFAULT '',
    can_invite_users BOOLEAN NOT NULL DEFAULT FALSE,
    can_restrict_members BOOLEAN NOT NULL DEFAULT FALSE,
    can_manage_chat BOOLEAN NOT NULL DEFAULT FALSE,
    last_seen_at TIMESTAMPTZ NOT NULL,
    last_verified_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_telegram_chats_status_seen
    ON telegram_chats (bot_status, last_seen_at DESC);

COMMENT ON TABLE telegram_chats IS 'Telegram chats discovered from bot membership updates for connector chat selection and invite-link generation.';
COMMENT ON COLUMN telegram_chats.chat_id IS 'Telegram chat ID in Bot API format, usually negative for groups/channels. Example: -1001234567890.';
COMMENT ON COLUMN telegram_chats.bot_status IS 'Bot membership status from my_chat_member update. Example: administrator, member, left, kicked.';

-- +migrate Down

DROP TABLE IF EXISTS telegram_chats;
