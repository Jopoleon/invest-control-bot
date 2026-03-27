-- +migrate Up
CREATE TABLE connectors (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    start_payload TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    chat_id TEXT NOT NULL,
    channel_url TEXT NOT NULL DEFAULT '',
    price_rub BIGINT NOT NULL,
    period_days INT NOT NULL,
    offer_url TEXT NOT NULL DEFAULT '',
    privacy_url TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    telegram_id BIGINT NULL,
    telegram_username TEXT NOT NULL DEFAULT '',
    full_name TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_users_telegram_id_unique ON users (telegram_id);

CREATE TABLE user_messenger_accounts (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    messenger_kind TEXT NOT NULL,
    external_user_id TEXT NOT NULL,
    username TEXT NOT NULL DEFAULT '',
    linked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (messenger_kind, external_user_id)
);

CREATE UNIQUE INDEX idx_user_messenger_accounts_user_kind
    ON user_messenger_accounts (user_id, messenger_kind);

CREATE TABLE user_settings (
    telegram_id BIGINT PRIMARY KEY,
    auto_pay_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE legal_documents (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    doc_type TEXT NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    external_url TEXT NOT NULL DEFAULT '',
    version INT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_legal_documents_type_version
    ON legal_documents (doc_type, version);

CREATE TABLE user_consents (
    telegram_id BIGINT NOT NULL,
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    offer_accepted_at TIMESTAMPTZ NOT NULL,
    privacy_accepted_at TIMESTAMPTZ NOT NULL,
    offer_document_id BIGINT NOT NULL DEFAULT 0,
    offer_document_version INT NOT NULL DEFAULT 0,
    privacy_document_id BIGINT NOT NULL DEFAULT 0,
    privacy_document_version INT NOT NULL DEFAULT 0,
    PRIMARY KEY (telegram_id, connector_id)
);

CREATE TABLE recurring_consents (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    telegram_id BIGINT NOT NULL,
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    accepted_at TIMESTAMPTZ NOT NULL,
    offer_document_id BIGINT NOT NULL DEFAULT 0,
    offer_document_version INT NOT NULL DEFAULT 0,
    user_agreement_document_id BIGINT NOT NULL DEFAULT 0,
    user_agreement_document_version INT NOT NULL DEFAULT 0
);

CREATE INDEX idx_recurring_consents_telegram_id
    ON recurring_consents (telegram_id, accepted_at DESC, id DESC);

CREATE TABLE registration_states (
    telegram_id BIGINT PRIMARY KEY,
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    step TEXT NOT NULL,
    telegram_username TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE payments (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider TEXT NOT NULL,
    provider_payment_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    token TEXT NOT NULL UNIQUE,
    user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    telegram_id BIGINT NOT NULL,
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    subscription_id BIGINT NOT NULL DEFAULT 0,
    parent_payment_id BIGINT NOT NULL DEFAULT 0,
    auto_pay_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    amount_rub BIGINT NOT NULL,
    checkout_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    paid_at TIMESTAMPTZ NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_payments_telegram_id ON payments (telegram_id);
CREATE INDEX idx_payments_user_id ON payments (user_id);
CREATE INDEX idx_payments_connector_id ON payments (connector_id);
CREATE INDEX idx_payments_subscription_id ON payments (subscription_id);
CREATE INDEX idx_payments_parent_payment_id ON payments (parent_payment_id);
CREATE INDEX idx_payments_status ON payments (status);
CREATE INDEX idx_payments_created_at ON payments (created_at DESC);
CREATE UNIQUE INDEX idx_payments_pending_rebill_subscription
    ON payments (subscription_id)
    WHERE subscription_id > 0 AND status = 'pending';

CREATE TABLE subscriptions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL,
    telegram_id BIGINT NOT NULL,
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    payment_id BIGINT NOT NULL UNIQUE REFERENCES payments(id),
    status TEXT NOT NULL,
    auto_pay_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    reminder_sent_at TIMESTAMPTZ NULL,
    expiry_notice_sent_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_subscriptions_telegram_id ON subscriptions (telegram_id);
CREATE INDEX idx_subscriptions_user_id ON subscriptions (user_id);
CREATE INDEX idx_subscriptions_connector_id ON subscriptions (connector_id);
CREATE INDEX idx_subscriptions_status ON subscriptions (status);
CREATE INDEX idx_subscriptions_ends_at ON subscriptions (ends_at);
CREATE INDEX idx_subscriptions_reminder_due
    ON subscriptions (status, ends_at)
    WHERE reminder_sent_at IS NULL;
CREATE INDEX idx_subscriptions_expiry_notice_due
    ON subscriptions (status, ends_at)
    WHERE expiry_notice_sent_at IS NULL;

CREATE TABLE audit_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    telegram_id BIGINT NOT NULL DEFAULT 0,
    connector_id BIGINT NOT NULL DEFAULT 0,
    action TEXT NOT NULL,
    details TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_events_created_at ON audit_events (created_at DESC);
CREATE INDEX idx_audit_events_telegram_id ON audit_events (telegram_id);
CREATE INDEX idx_audit_events_connector_id ON audit_events (connector_id);

CREATE TABLE admin_sessions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    session_token_hash TEXT NOT NULL UNIQUE,
    subject TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    rotated_at TIMESTAMPTZ NULL,
    replaced_by_hash TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_admin_sessions_expires_at ON admin_sessions (expires_at);
CREATE INDEX idx_admin_sessions_last_seen_at ON admin_sessions (last_seen_at);
CREATE INDEX idx_admin_sessions_revoked_at ON admin_sessions (revoked_at);

-- +migrate Down
DROP TABLE IF EXISTS admin_sessions;
DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS registration_states;
DROP TABLE IF EXISTS recurring_consents;
DROP TABLE IF EXISTS user_consents;
DROP TABLE IF EXISTS legal_documents;
DROP TABLE IF EXISTS user_settings;
DROP TABLE IF EXISTS user_messenger_accounts;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS connectors;
