-- +migrate Up

ALTER TABLE connectors
    ADD COLUMN IF NOT EXISTS max_channel_url TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN connectors.chat_id IS 'Telegram chat or channel join target used for invite link generation or fallback URL building. Example: @invest_channel or -1001234567890.';
COMMENT ON COLUMN connectors.channel_url IS 'Explicit Telegram channel link shown to Telegram users after payment. Example: https://t.me/testtestinvest.';
COMMENT ON COLUMN connectors.max_channel_url IS 'Explicit MAX destination link shown to MAX users after payment. Example: https://max.ru/-72598909498032.';

-- +migrate Down

ALTER TABLE connectors
    DROP COLUMN IF EXISTS max_channel_url;
