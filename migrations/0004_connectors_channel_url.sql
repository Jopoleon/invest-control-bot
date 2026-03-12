-- +migrate Up
ALTER TABLE connectors
    ADD COLUMN IF NOT EXISTS channel_url TEXT NOT NULL DEFAULT '';

-- +migrate Down
ALTER TABLE connectors
    DROP COLUMN IF EXISTS channel_url;
