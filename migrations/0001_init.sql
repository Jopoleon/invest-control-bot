-- +migrate Up
CREATE TABLE IF NOT EXISTS connectors (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    start_payload TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    chat_id TEXT NOT NULL,
    price_rub BIGINT NOT NULL,
    period_days INT NOT NULL,
    offer_url TEXT NOT NULL DEFAULT '',
    privacy_url TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
    telegram_id BIGINT PRIMARY KEY,
    telegram_username TEXT NOT NULL DEFAULT '',
    full_name TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_consents (
    telegram_id BIGINT NOT NULL,
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    offer_accepted_at TIMESTAMPTZ NOT NULL,
    privacy_accepted_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (telegram_id, connector_id)
);

CREATE TABLE IF NOT EXISTS registration_states (
    telegram_id BIGINT PRIMARY KEY,
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    step TEXT NOT NULL,
    telegram_username TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +migrate Down
DROP TABLE IF EXISTS registration_states;
DROP TABLE IF EXISTS user_consents;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS connectors;
