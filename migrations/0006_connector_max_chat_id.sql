-- +migrate Up

ALTER TABLE connectors
    ADD COLUMN IF NOT EXISTS max_chat_id TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN connectors.max_chat_id IS 'Explicit MAX group chat ID used for membership revoke/regrant operations. Example: -72598909498032.';

-- +migrate Down

ALTER TABLE connectors
    DROP COLUMN IF EXISTS max_chat_id;
