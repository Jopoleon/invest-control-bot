-- +migrate Up
ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;

UPDATE payments p
SET user_id = u.id
FROM users u
WHERE p.user_id IS NULL
  AND u.telegram_id = p.telegram_id;

UPDATE subscriptions s
SET user_id = u.id
FROM users u
WHERE s.user_id IS NULL
  AND u.telegram_id = s.telegram_id;

CREATE INDEX IF NOT EXISTS idx_payments_user_id ON payments (user_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_user_id ON subscriptions (user_id);

-- +migrate Down
DROP INDEX IF EXISTS idx_subscriptions_user_id;
DROP INDEX IF EXISTS idx_payments_user_id;

ALTER TABLE subscriptions
    DROP COLUMN IF EXISTS user_id;

ALTER TABLE payments
    DROP COLUMN IF EXISTS user_id;
