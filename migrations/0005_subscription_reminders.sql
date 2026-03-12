-- +migrate Up
ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS reminder_sent_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_subscriptions_reminder_due
    ON subscriptions (status, ends_at)
    WHERE reminder_sent_at IS NULL;

-- +migrate Down
DROP INDEX IF EXISTS idx_subscriptions_reminder_due;

ALTER TABLE subscriptions
    DROP COLUMN IF EXISTS reminder_sent_at;
