-- +migrate Up
ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS subscription_id BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS parent_payment_id BIGINT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_payments_subscription_id ON payments (subscription_id);
CREATE INDEX IF NOT EXISTS idx_payments_parent_payment_id ON payments (parent_payment_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_payments_pending_rebill_subscription
    ON payments (subscription_id)
    WHERE subscription_id > 0 AND status = 'pending';

ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS expiry_notice_sent_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_subscriptions_expiry_notice_due
    ON subscriptions (status, ends_at)
    WHERE expiry_notice_sent_at IS NULL;

-- +migrate Down
DROP INDEX IF EXISTS idx_subscriptions_expiry_notice_due;

ALTER TABLE subscriptions
    DROP COLUMN IF EXISTS expiry_notice_sent_at;

DROP INDEX IF EXISTS idx_payments_pending_rebill_subscription;
DROP INDEX IF EXISTS idx_payments_parent_payment_id;
DROP INDEX IF EXISTS idx_payments_subscription_id;

ALTER TABLE payments
    DROP COLUMN IF EXISTS parent_payment_id,
    DROP COLUMN IF EXISTS subscription_id;
