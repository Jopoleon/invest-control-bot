-- +migrate Up

-- connectors stores sellable tariff entry points.
-- One connector represents one tariff / checkout entry, not one user.
CREATE TABLE connectors (
    -- Internal connector id. Example: 13.
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- Bot deep-link payload used in /start flow. Example: in-6ae23f91735bfab1.
    start_payload TEXT NOT NULL UNIQUE,
    -- Human-readable tariff name shown in bot and admin. Example: MAX Premium March.
    name TEXT NOT NULL,
    -- Optional marketing or support description shown before checkout. Example: Monthly access to MAX test channel.
    description TEXT NOT NULL DEFAULT '',
    -- Transport-specific access target or join key. Example: @invest_channel or -1001234567890.
    chat_id TEXT NOT NULL,
    -- Explicit public channel link shown to user after payment. Example: https://web.max.ru/-72598909498032.
    channel_url TEXT NOT NULL DEFAULT '',
    -- Price in RUB stored as whole integer amount. Example: 2300.
    price_rub BIGINT NOT NULL,
    -- Billing period in days. Example: 30.
    period_days INT NOT NULL,
    -- Optional connector-specific offer URL override. Example: https://site.example/oferta/8.
    offer_url TEXT NOT NULL DEFAULT '',
    -- Optional connector-specific privacy URL override. Example: https://site.example/policy/9.
    privacy_url TEXT NOT NULL DEFAULT '',
    -- Soft switch for selling or hiding the connector. Example: TRUE.
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    -- Connector creation timestamp in UTC. Example: 2026-03-27T09:00:00Z.
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE connectors IS 'Sellable tariff entry points keyed by bot start payload.';
COMMENT ON COLUMN connectors.id IS 'Internal connector id. Example: 13.';
COMMENT ON COLUMN connectors.start_payload IS 'Bot deep-link payload used in /start flow. Example: in-6ae23f91735bfab1.';
COMMENT ON COLUMN connectors.name IS 'Human-readable tariff name shown in bot and admin. Example: MAX Premium March.';
COMMENT ON COLUMN connectors.description IS 'Optional marketing or support description shown before checkout. Example: Monthly access to MAX test channel.';
COMMENT ON COLUMN connectors.chat_id IS 'Transport-specific access target or join key. Example: @invest_channel or -1001234567890.';
COMMENT ON COLUMN connectors.channel_url IS 'Explicit public channel link shown to user after payment. Example: https://web.max.ru/-72598909498032.';
COMMENT ON COLUMN connectors.price_rub IS 'Price in RUB stored as whole integer amount. Example: 2300.';
COMMENT ON COLUMN connectors.period_days IS 'Billing period in days. Example: 30.';
COMMENT ON COLUMN connectors.offer_url IS 'Optional connector-specific offer URL override. Example: https://site.example/oferta/8.';
COMMENT ON COLUMN connectors.privacy_url IS 'Optional connector-specific privacy URL override. Example: https://site.example/policy/9.';
COMMENT ON COLUMN connectors.is_active IS 'Soft switch for selling or hiding the connector. Example: TRUE.';
COMMENT ON COLUMN connectors.created_at IS 'Connector creation timestamp in UTC. Example: 2026-03-27T09:00:00Z.';

-- users stores one internal person record shared across all linked channels.
CREATE TABLE users (
    -- Internal canonical user id. Example: 42.
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- Full name collected during onboarding. Example: Ivan Ivanov.
    full_name TEXT NOT NULL DEFAULT '',
    -- Phone number collected during onboarding. Example: +79991234567.
    phone TEXT NOT NULL DEFAULT '',
    -- Email collected during onboarding. Example: ivan@example.com.
    email TEXT NOT NULL DEFAULT '',
    -- User creation timestamp. Example: 2026-03-27T09:05:00Z.
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Last profile update timestamp. Example: 2026-03-27T09:06:00Z.
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE users IS 'Canonical internal user records shared across messengers.';
COMMENT ON COLUMN users.id IS 'Internal canonical user id. Example: 42.';
COMMENT ON COLUMN users.full_name IS 'Full name collected during onboarding. Example: Ivan Ivanov.';
COMMENT ON COLUMN users.phone IS 'Phone number collected during onboarding. Example: +79991234567.';
COMMENT ON COLUMN users.email IS 'Email collected during onboarding. Example: ivan@example.com.';
COMMENT ON COLUMN users.created_at IS 'User creation timestamp. Example: 2026-03-27T09:05:00Z.';
COMMENT ON COLUMN users.updated_at IS 'Last profile update timestamp. Example: 2026-03-27T09:06:00Z.';

-- user_messenger_accounts links one internal user to one messenger account.
-- One row represents one account in one external messenger.
CREATE TABLE user_messenger_accounts (
    -- Internal user owner. Example: 42.
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Messenger kind discriminator. Example: telegram or max.
    messenger_kind TEXT NOT NULL,
    -- Messenger-specific account id stored as text for cross-platform compatibility. Example: 264704572 or 193465776.
    messenger_user_id TEXT NOT NULL,
    -- Latest known username in that messenger. Example: emiloserdov.
    username TEXT NOT NULL DEFAULT '',
    -- First link timestamp between internal user and messenger account. Example: 2026-03-27T09:10:00Z.
    linked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Last metadata refresh timestamp for this messenger account. Example: 2026-03-27T09:11:00Z.
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (messenger_kind, messenger_user_id)
);

COMMENT ON TABLE user_messenger_accounts IS 'Normalized messenger account identities linked to one internal user.';
COMMENT ON COLUMN user_messenger_accounts.user_id IS 'Internal user owner. Example: 42.';
COMMENT ON COLUMN user_messenger_accounts.messenger_kind IS 'Messenger kind discriminator. Example: telegram or max.';
COMMENT ON COLUMN user_messenger_accounts.messenger_user_id IS 'Messenger-specific account id stored as text for cross-platform compatibility. Example: 264704572 or 193465776.';
COMMENT ON COLUMN user_messenger_accounts.username IS 'Latest known username in that messenger. Example: emiloserdov.';
COMMENT ON COLUMN user_messenger_accounts.linked_at IS 'First link timestamp between internal user and messenger account. Example: 2026-03-27T09:10:00Z.';
COMMENT ON COLUMN user_messenger_accounts.updated_at IS 'Last metadata refresh timestamp for this messenger account. Example: 2026-03-27T09:11:00Z.';

CREATE UNIQUE INDEX idx_user_messenger_accounts_user_kind
    ON user_messenger_accounts (user_id, messenger_kind);

-- legal_documents stores versioned legal texts or external links.
CREATE TABLE legal_documents (
    -- Internal legal document id. Example: 8.
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- Document type discriminator. Example: offer, privacy, user_agreement.
    doc_type TEXT NOT NULL,
    -- Human-readable document title. Example: Public Offer Agreement.
    title TEXT NOT NULL,
    -- Inline document body when stored in DB. Example: full legal text html or plain text.
    content TEXT NOT NULL DEFAULT '',
    -- External public URL override. Example: https://site.example/oferta/8.
    external_url TEXT NOT NULL DEFAULT '',
    -- Version number within one document type. Example: 3.
    version INT NOT NULL,
    -- Published flag used by public pages and bot. Example: TRUE.
    is_active BOOLEAN NOT NULL DEFAULT FALSE,
    -- Creation timestamp for this version. Example: 2026-03-27T09:15:00Z.
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE legal_documents IS 'Versioned legal texts or external legal URLs used in onboarding and recurring consent.';
COMMENT ON COLUMN legal_documents.id IS 'Internal legal document id. Example: 8.';
COMMENT ON COLUMN legal_documents.doc_type IS 'Document type discriminator. Example: offer, privacy, user_agreement.';
COMMENT ON COLUMN legal_documents.title IS 'Human-readable document title. Example: Public Offer Agreement.';
COMMENT ON COLUMN legal_documents.content IS 'Inline document body when stored in DB. Example: full legal text.';
COMMENT ON COLUMN legal_documents.external_url IS 'External public URL override. Example: https://site.example/oferta/8.';
COMMENT ON COLUMN legal_documents.version IS 'Version number within one document type. Example: 3.';
COMMENT ON COLUMN legal_documents.is_active IS 'Published flag used by public pages and bot. Example: TRUE.';
COMMENT ON COLUMN legal_documents.created_at IS 'Creation timestamp for this version. Example: 2026-03-27T09:15:00Z.';

CREATE UNIQUE INDEX idx_legal_documents_type_version
    ON legal_documents (doc_type, version);

-- user_consents stores ordinary offer and privacy acceptance facts.
CREATE TABLE user_consents (
    -- Internal user that accepted the legal bundle. Example: 42.
    user_id BIGINT NOT NULL REFERENCES users(id),
    -- Connector for which the consent was accepted. Example: 13.
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    -- Timestamp of offer acceptance. Example: 2026-03-27T09:20:00Z.
    offer_accepted_at TIMESTAMPTZ NOT NULL,
    -- Timestamp of privacy acceptance. Example: 2026-03-27T09:20:05Z.
    privacy_accepted_at TIMESTAMPTZ NOT NULL,
    -- Resolved legal_documents.id for accepted offer version, or 0 when external link only. Example: 8.
    offer_document_id BIGINT NOT NULL DEFAULT 0,
    -- Resolved offer version accepted by user, or 0 when unknown. Example: 3.
    offer_document_version INT NOT NULL DEFAULT 0,
    -- Resolved legal_documents.id for accepted privacy version, or 0 when external link only. Example: 9.
    privacy_document_id BIGINT NOT NULL DEFAULT 0,
    -- Resolved privacy version accepted by user, or 0 when unknown. Example: 2.
    privacy_document_version INT NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, connector_id)
);

COMMENT ON TABLE user_consents IS 'Ordinary offer and privacy acceptance facts per internal user and connector.';
COMMENT ON COLUMN user_consents.user_id IS 'Internal user that accepted the legal bundle. Example: 42.';
COMMENT ON COLUMN user_consents.connector_id IS 'Connector for which the consent was accepted. Example: 13.';
COMMENT ON COLUMN user_consents.offer_accepted_at IS 'Timestamp of offer acceptance. Example: 2026-03-27T09:20:00Z.';
COMMENT ON COLUMN user_consents.privacy_accepted_at IS 'Timestamp of privacy acceptance. Example: 2026-03-27T09:20:05Z.';
COMMENT ON COLUMN user_consents.offer_document_id IS 'Resolved legal document id for accepted offer version, or 0 when external link only. Example: 8.';
COMMENT ON COLUMN user_consents.offer_document_version IS 'Resolved offer version accepted by user, or 0 when unknown. Example: 3.';
COMMENT ON COLUMN user_consents.privacy_document_id IS 'Resolved legal document id for accepted privacy version, or 0 when external link only. Example: 9.';
COMMENT ON COLUMN user_consents.privacy_document_version IS 'Resolved privacy version accepted by user, or 0 when unknown. Example: 2.';

-- recurring_consents stores explicit opt-in history for recurring charges.
CREATE TABLE recurring_consents (
    -- Internal recurring consent id. Example: 17.
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- Internal user that explicitly enabled recurring charges. Example: 42.
    user_id BIGINT NOT NULL REFERENCES users(id),
    -- Connector for which recurring was enabled. Example: 13.
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    -- Timestamp of explicit recurring consent. Example: 2026-03-27T09:30:00Z.
    accepted_at TIMESTAMPTZ NOT NULL,
    -- Resolved offer document id used for recurring legal bundle. Example: 8.
    offer_document_id BIGINT NOT NULL DEFAULT 0,
    -- Resolved offer version used for recurring legal bundle. Example: 3.
    offer_document_version INT NOT NULL DEFAULT 0,
    -- Resolved user agreement document id. Example: 10.
    user_agreement_document_id BIGINT NOT NULL DEFAULT 0,
    -- Resolved user agreement version. Example: 1.
    user_agreement_document_version INT NOT NULL DEFAULT 0
);

COMMENT ON TABLE recurring_consents IS 'Explicit opt-in history for recurring charges.';
COMMENT ON COLUMN recurring_consents.id IS 'Internal recurring consent id. Example: 17.';
COMMENT ON COLUMN recurring_consents.user_id IS 'Internal user that explicitly enabled recurring charges. Example: 42.';
COMMENT ON COLUMN recurring_consents.connector_id IS 'Connector for which recurring was enabled. Example: 13.';
COMMENT ON COLUMN recurring_consents.accepted_at IS 'Timestamp of explicit recurring consent. Example: 2026-03-27T09:30:00Z.';
COMMENT ON COLUMN recurring_consents.offer_document_id IS 'Resolved offer document id used for recurring legal bundle. Example: 8.';
COMMENT ON COLUMN recurring_consents.offer_document_version IS 'Resolved offer version used for recurring legal bundle. Example: 3.';
COMMENT ON COLUMN recurring_consents.user_agreement_document_id IS 'Resolved user agreement document id. Example: 10.';
COMMENT ON COLUMN recurring_consents.user_agreement_document_version IS 'Resolved user agreement version. Example: 1.';

CREATE INDEX idx_recurring_consents_user_id
    ON recurring_consents (user_id, accepted_at DESC, id DESC);

-- registration_states stores in-progress onboarding FSM state per messenger account.
CREATE TABLE registration_states (
    -- Messenger kind for the in-progress registration. Example: telegram or max.
    messenger_kind TEXT NOT NULL,
    -- Messenger-specific user id used as FSM key. Example: 264704572 or 193465776.
    messenger_user_id TEXT NOT NULL,
    -- Connector the user is currently registering for. Example: 13.
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    -- Current FSM step. Example: full_name, phone, email.
    step TEXT NOT NULL,
    -- Latest known username seen in that messenger. Example: emiloserdov.
    username TEXT NOT NULL DEFAULT '',
    -- Last FSM update timestamp. Example: 2026-03-27T09:40:00Z.
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (messenger_kind, messenger_user_id)
);

COMMENT ON TABLE registration_states IS 'In-progress onboarding FSM state keyed by messenger account.';
COMMENT ON COLUMN registration_states.messenger_kind IS 'Messenger kind for the in-progress registration. Example: telegram or max.';
COMMENT ON COLUMN registration_states.messenger_user_id IS 'Messenger-specific user id used as FSM key. Example: 264704572 or 193465776.';
COMMENT ON COLUMN registration_states.connector_id IS 'Connector the user is currently registering for. Example: 13.';
COMMENT ON COLUMN registration_states.step IS 'Current FSM step. Example: full_name, phone, email.';
COMMENT ON COLUMN registration_states.username IS 'Latest known username seen in that messenger. Example: emiloserdov.';
COMMENT ON COLUMN registration_states.updated_at IS 'Last FSM update timestamp. Example: 2026-03-27T09:40:00Z.';

-- payments stores checkout attempts and recurring rebill records.
CREATE TABLE payments (
    -- Internal payment id. Example: 14.
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- Payment provider name. Example: robokassa.
    provider TEXT NOT NULL,
    -- Provider-side payment id after callback confirmation. Example: robokassa:8094871605448087949.
    provider_payment_id TEXT NOT NULL DEFAULT '',
    -- Internal payment status. Example: pending, paid, failed.
    status TEXT NOT NULL,
    -- Public opaque token used in redirect or internal payment lookup. Example: 8094871605448087949.
    token TEXT NOT NULL UNIQUE,
    -- Canonical internal user owner. Example: 42.
    user_id BIGINT NOT NULL REFERENCES users(id),
    -- Connector being purchased. Example: 13.
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    -- Target subscription id for rebill attempts, or 0 for first payment. Example: 12.
    subscription_id BIGINT NOT NULL DEFAULT 0,
    -- Parent payment id for recurring chains, or 0 for first payment. Example: 14.
    parent_payment_id BIGINT NOT NULL DEFAULT 0,
    -- Snapshot of recurring choice on this payment. Example: TRUE.
    auto_pay_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    -- Charged amount in RUB. Example: 6666.
    amount_rub BIGINT NOT NULL,
    -- Checkout URL returned by provider. Example: https://auth.robokassa.ru/Merchant/Index.aspx?... .
    checkout_url TEXT NOT NULL DEFAULT '',
    -- Payment creation timestamp. Example: 2026-03-27T09:50:00Z.
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Provider-confirmed paid timestamp, null until success. Example: 2026-03-27T09:55:00Z.
    paid_at TIMESTAMPTZ NULL,
    -- Last payment row update timestamp. Example: 2026-03-27T09:55:00Z.
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE payments IS 'Checkout attempts and recurring rebill records.';
COMMENT ON COLUMN payments.id IS 'Internal payment id. Example: 14.';
COMMENT ON COLUMN payments.provider IS 'Payment provider name. Example: robokassa.';
COMMENT ON COLUMN payments.provider_payment_id IS 'Provider-side payment id after confirmation. For Robokassa this is external operation context such as OpKey or a derived provider marker. Example: robokassa:8094871605448087949.';
COMMENT ON COLUMN payments.status IS 'Internal payment status. Example: pending, paid, failed.';
COMMENT ON COLUMN payments.token IS 'Merchant-side payment reference used across checkout, callbacks and internal lookup. For Robokassa this field stores InvoiceID / InvId. Example: 8094871605448087949.';
COMMENT ON COLUMN payments.user_id IS 'Canonical internal user owner. Example: 42.';
COMMENT ON COLUMN payments.connector_id IS 'Connector being purchased. Example: 13.';
COMMENT ON COLUMN payments.subscription_id IS 'Target subscription id for rebill attempts, or 0 for first payment. Example: 12.';
COMMENT ON COLUMN payments.parent_payment_id IS 'Parent payment id for recurring chains, or 0 for first payment. Example: 14.';
COMMENT ON COLUMN payments.auto_pay_enabled IS 'Snapshot of recurring choice on this payment. Example: TRUE.';
COMMENT ON COLUMN payments.amount_rub IS 'Charged amount in RUB. Example: 6666.';
COMMENT ON COLUMN payments.checkout_url IS 'Checkout URL returned by provider. Example: https://auth.robokassa.ru/Merchant/Index.aspx?... .';
COMMENT ON COLUMN payments.created_at IS 'Payment creation timestamp. Example: 2026-03-27T09:50:00Z.';
COMMENT ON COLUMN payments.paid_at IS 'Provider-confirmed paid timestamp, null until success. Example: 2026-03-27T09:55:00Z.';
COMMENT ON COLUMN payments.updated_at IS 'Last payment row update timestamp. Example: 2026-03-27T09:55:00Z.';

CREATE INDEX idx_payments_user_id ON payments (user_id);
CREATE INDEX idx_payments_connector_id ON payments (connector_id);
CREATE INDEX idx_payments_subscription_id ON payments (subscription_id);
CREATE INDEX idx_payments_parent_payment_id ON payments (parent_payment_id);
CREATE INDEX idx_payments_status ON payments (status);
CREATE INDEX idx_payments_created_at ON payments (created_at DESC);
CREATE UNIQUE INDEX idx_payments_pending_rebill_subscription
    ON payments (subscription_id)
    WHERE subscription_id > 0 AND status = 'pending';

-- subscriptions stores actual paid access periods.
CREATE TABLE subscriptions (
    -- Internal subscription id. Example: 12.
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- Canonical internal user owner. Example: 42.
    user_id BIGINT NOT NULL REFERENCES users(id),
    -- Connector that defines tariff and access target. Example: 13.
    connector_id BIGINT NOT NULL REFERENCES connectors(id),
    -- Successful originating payment id. Example: 14.
    payment_id BIGINT NOT NULL UNIQUE REFERENCES payments(id),
    -- Subscription status. Example: active, expired, revoked.
    status TEXT NOT NULL,
    -- Canonical per-subscription recurring flag. Example: TRUE.
    auto_pay_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    -- Access period start timestamp. Example: 2026-03-27T10:00:00Z.
    starts_at TIMESTAMPTZ NOT NULL,
    -- Access period end timestamp. Example: 2026-04-26T10:00:00Z.
    ends_at TIMESTAMPTZ NOT NULL,
    -- Reminder delivery timestamp, null until sent. Example: 2026-04-23T10:00:00Z.
    reminder_sent_at TIMESTAMPTZ NULL,
    -- Same-day expiry notice timestamp, null until sent. Example: 2026-04-26T08:00:00Z.
    expiry_notice_sent_at TIMESTAMPTZ NULL,
    -- Subscription creation timestamp. Example: 2026-03-27T10:00:00Z.
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Last subscription row update timestamp. Example: 2026-03-27T10:00:00Z.
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE subscriptions IS 'Paid access periods linked to successful payments.';
COMMENT ON COLUMN subscriptions.id IS 'Internal subscription id. Example: 12.';
COMMENT ON COLUMN subscriptions.user_id IS 'Canonical internal user owner. Example: 42.';
COMMENT ON COLUMN subscriptions.connector_id IS 'Connector that defines tariff and access target. Example: 13.';
COMMENT ON COLUMN subscriptions.payment_id IS 'Successful originating payment id. Example: 14.';
COMMENT ON COLUMN subscriptions.status IS 'Subscription status. Example: active, expired, revoked.';
COMMENT ON COLUMN subscriptions.auto_pay_enabled IS 'Canonical per-subscription recurring flag. Example: TRUE.';
COMMENT ON COLUMN subscriptions.starts_at IS 'Access period start timestamp. Example: 2026-03-27T10:00:00Z.';
COMMENT ON COLUMN subscriptions.ends_at IS 'Access period end timestamp. Example: 2026-04-26T10:00:00Z.';
COMMENT ON COLUMN subscriptions.reminder_sent_at IS 'Reminder delivery timestamp, null until sent. Example: 2026-04-23T10:00:00Z.';
COMMENT ON COLUMN subscriptions.expiry_notice_sent_at IS 'Same-day expiry notice timestamp, null until sent. Example: 2026-04-26T08:00:00Z.';
COMMENT ON COLUMN subscriptions.created_at IS 'Subscription creation timestamp. Example: 2026-03-27T10:00:00Z.';
COMMENT ON COLUMN subscriptions.updated_at IS 'Last subscription row update timestamp. Example: 2026-03-27T10:00:00Z.';

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

-- audit_events stores immutable audit records with explicit actor and target context.
CREATE TABLE audit_events (
    -- Internal audit event id. Example: 105.
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- Origin of the action. Example: user, admin or app.
    actor_type TEXT NOT NULL,
    -- Internal user who initiated the action, if any. Example: 42.
    actor_user_id BIGINT NULL REFERENCES users(id),
    -- Messenger kind used by the actor when the event came from a messenger. Example: telegram or max.
    actor_messenger_kind TEXT NOT NULL DEFAULT '',
    -- Messenger-specific id snapshot for the actor. Example: 264704572 or 193465776.
    actor_messenger_user_id TEXT NOT NULL DEFAULT '',
    -- Free-form actor label when there is no concrete user account. Example: admin_panel.
    actor_subject TEXT NOT NULL DEFAULT '',
    -- Internal user affected by the action, if any. Example: 42.
    target_user_id BIGINT NULL REFERENCES users(id),
    -- Messenger kind of the affected account, if any. Example: telegram or max.
    target_messenger_kind TEXT NOT NULL DEFAULT '',
    -- Messenger-specific id of the affected account, if any. Example: 264704572 or 193465776.
    target_messenger_user_id TEXT NOT NULL DEFAULT '',
    -- Connector associated with the event, if any. Example: 13.
    connector_id BIGINT NULL REFERENCES connectors(id),
    -- Audit action name. Example: payment_created or admin_login_success.
    action TEXT NOT NULL,
    -- Free-form details string for support and debugging. Example: token=8094871605448087949.
    details TEXT NOT NULL DEFAULT '',
    -- Event creation timestamp. Example: 2026-03-27T10:05:00Z.
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE audit_events IS 'Immutable audit records with explicit actor and target context.';
COMMENT ON COLUMN audit_events.id IS 'Internal audit event id. Example: 105.';
COMMENT ON COLUMN audit_events.actor_type IS 'Origin of the action. Example: user, admin or app.';
COMMENT ON COLUMN audit_events.actor_user_id IS 'Internal user who initiated the action, if any. Example: 42.';
COMMENT ON COLUMN audit_events.actor_messenger_kind IS 'Messenger kind used by the actor when the event came from a messenger. Example: telegram or max.';
COMMENT ON COLUMN audit_events.actor_messenger_user_id IS 'Messenger-specific id snapshot for the actor. Example: 264704572 or 193465776.';
COMMENT ON COLUMN audit_events.actor_subject IS 'Free-form actor label when there is no concrete user account. Example: admin_panel.';
COMMENT ON COLUMN audit_events.target_user_id IS 'Internal user affected by the action, if any. Example: 42.';
COMMENT ON COLUMN audit_events.target_messenger_kind IS 'Messenger kind of the affected account, if any. Example: telegram or max.';
COMMENT ON COLUMN audit_events.target_messenger_user_id IS 'Messenger-specific id of the affected account, if any. Example: 264704572 or 193465776.';
COMMENT ON COLUMN audit_events.connector_id IS 'Connector associated with the event, if any. Example: 13.';
COMMENT ON COLUMN audit_events.action IS 'Audit action name. Example: payment_created or admin_login_success.';
COMMENT ON COLUMN audit_events.details IS 'Free-form details string for support and debugging. Example: token=8094871605448087949.';
COMMENT ON COLUMN audit_events.created_at IS 'Event creation timestamp. Example: 2026-03-27T10:05:00Z.';

CREATE INDEX idx_audit_events_created_at ON audit_events (created_at DESC);
CREATE INDEX idx_audit_events_actor_type ON audit_events (actor_type, created_at DESC);
CREATE INDEX idx_audit_events_target_user_id ON audit_events (target_user_id);
CREATE INDEX idx_audit_events_target_messenger_identity
    ON audit_events (target_messenger_kind, target_messenger_user_id);
CREATE INDEX idx_audit_events_connector_id ON audit_events (connector_id);

-- admin_sessions stores browser sessions for admin panel access.
CREATE TABLE admin_sessions (
    -- Internal admin session id. Example: 3.
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- Hash of admin session token stored server-side. Example: sha256:abcd1234.
    session_token_hash TEXT NOT NULL UNIQUE,
    -- Session subject or principal label. Example: admin.
    subject TEXT NOT NULL,
    -- Session creation timestamp. Example: 2026-03-27T10:10:00Z.
    created_at TIMESTAMPTZ NOT NULL,
    -- Absolute session expiry timestamp. Example: 2026-03-28T10:10:00Z.
    expires_at TIMESTAMPTZ NOT NULL,
    -- Last-seen timestamp for rolling activity display. Example: 2026-03-27T10:15:00Z.
    last_seen_at TIMESTAMPTZ NOT NULL,
    -- Revocation timestamp, null while session is active. Example: 2026-03-27T11:00:00Z.
    revoked_at TIMESTAMPTZ NULL,
    -- Client IP captured on creation or rotation. Example: 127.0.0.1.
    ip TEXT NOT NULL DEFAULT '',
    -- Client user agent string. Example: Mozilla/5.0.
    user_agent TEXT NOT NULL DEFAULT '',
    -- Rotation timestamp when token was rotated. Example: 2026-03-27T12:00:00Z.
    rotated_at TIMESTAMPTZ NULL,
    -- Hash of replacement token after rotation. Example: sha256:nexttokenhash.
    replaced_by_hash TEXT NOT NULL DEFAULT ''
);

COMMENT ON TABLE admin_sessions IS 'Browser sessions for admin panel access.';
COMMENT ON COLUMN admin_sessions.id IS 'Internal admin session id. Example: 3.';
COMMENT ON COLUMN admin_sessions.session_token_hash IS 'Hash of admin session token stored server-side. Example: sha256:abcd1234.';
COMMENT ON COLUMN admin_sessions.subject IS 'Session subject or principal label. Example: admin.';
COMMENT ON COLUMN admin_sessions.created_at IS 'Session creation timestamp. Example: 2026-03-27T10:10:00Z.';
COMMENT ON COLUMN admin_sessions.expires_at IS 'Absolute session expiry timestamp. Example: 2026-03-28T10:10:00Z.';
COMMENT ON COLUMN admin_sessions.last_seen_at IS 'Last-seen timestamp for rolling activity display. Example: 2026-03-27T10:15:00Z.';
COMMENT ON COLUMN admin_sessions.revoked_at IS 'Revocation timestamp, null while session is active. Example: 2026-03-27T11:00:00Z.';
COMMENT ON COLUMN admin_sessions.ip IS 'Client IP captured on creation or rotation. Example: 127.0.0.1.';
COMMENT ON COLUMN admin_sessions.user_agent IS 'Client user agent string. Example: Mozilla/5.0.';
COMMENT ON COLUMN admin_sessions.rotated_at IS 'Rotation timestamp when token was rotated. Example: 2026-03-27T12:00:00Z.';
COMMENT ON COLUMN admin_sessions.replaced_by_hash IS 'Hash of replacement token after rotation. Example: sha256:nexttokenhash.';

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
DROP TABLE IF EXISTS user_messenger_accounts;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS connectors;
