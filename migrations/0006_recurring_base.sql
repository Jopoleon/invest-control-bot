-- +migrate Up
ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS auto_pay_enabled BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS auto_pay_enabled BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS user_settings (
    telegram_id BIGINT PRIMARY KEY,
    auto_pay_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +migrate Down
DROP TABLE IF EXISTS user_settings;

ALTER TABLE subscriptions
    DROP COLUMN IF EXISTS auto_pay_enabled;

ALTER TABLE payments
    DROP COLUMN IF EXISTS auto_pay_enabled;
