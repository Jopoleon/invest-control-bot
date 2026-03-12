-- +migrate Up
CREATE TABLE IF NOT EXISTS payments (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider TEXT NOT NULL,
    provider_payment_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    token TEXT NOT NULL UNIQUE,
    telegram_id BIGINT NOT NULL,
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    amount_rub BIGINT NOT NULL,
    checkout_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    paid_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_payments_telegram_id ON payments (telegram_id);
CREATE INDEX IF NOT EXISTS idx_payments_connector_id ON payments (connector_id);
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments (status);
CREATE INDEX IF NOT EXISTS idx_payments_created_at ON payments (created_at DESC);

CREATE TABLE IF NOT EXISTS subscriptions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    telegram_id BIGINT NOT NULL,
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    payment_id BIGINT NOT NULL UNIQUE REFERENCES payments(id),
    status TEXT NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_telegram_id ON subscriptions (telegram_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_connector_id ON subscriptions (connector_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_status ON subscriptions (status);
CREATE INDEX IF NOT EXISTS idx_subscriptions_ends_at ON subscriptions (ends_at);

-- +migrate Down
DROP INDEX IF EXISTS idx_subscriptions_ends_at;
DROP INDEX IF EXISTS idx_subscriptions_status;
DROP INDEX IF EXISTS idx_subscriptions_connector_id;
DROP INDEX IF EXISTS idx_subscriptions_telegram_id;
DROP TABLE IF EXISTS subscriptions;

DROP INDEX IF EXISTS idx_payments_created_at;
DROP INDEX IF EXISTS idx_payments_status;
DROP INDEX IF EXISTS idx_payments_connector_id;
DROP INDEX IF EXISTS idx_payments_telegram_id;
DROP TABLE IF EXISTS payments;
