-- +migrate Up
CREATE SEQUENCE IF NOT EXISTS users_id_seq;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS id BIGINT;

ALTER TABLE users
    ALTER COLUMN id SET DEFAULT nextval('users_id_seq');

UPDATE users
SET id = nextval('users_id_seq')
WHERE id IS NULL;

SELECT setval('users_id_seq', COALESCE((SELECT MAX(id) FROM users), 1), true);

ALTER TABLE users
    ALTER COLUMN id SET NOT NULL;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_pkey;

ALTER TABLE users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

ALTER TABLE users
    ALTER COLUMN telegram_id DROP NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_telegram_id_unique
    ON users (telegram_id);

CREATE TABLE IF NOT EXISTS user_messenger_accounts (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    messenger_kind TEXT NOT NULL,
    external_user_id TEXT NOT NULL,
    username TEXT NOT NULL DEFAULT '',
    linked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (messenger_kind, external_user_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_messenger_accounts_user_kind
    ON user_messenger_accounts (user_id, messenger_kind);

INSERT INTO user_messenger_accounts (
    user_id, messenger_kind, external_user_id, username, linked_at, updated_at
)
SELECT
    id,
    'telegram',
    CAST(telegram_id AS TEXT),
    telegram_username,
    NOW(),
    updated_at
FROM users
WHERE telegram_id IS NOT NULL
ON CONFLICT (messenger_kind, external_user_id) DO NOTHING;

-- +migrate Down
DELETE FROM users
WHERE telegram_id IS NULL;

DROP INDEX IF EXISTS idx_user_messenger_accounts_user_kind;
DROP TABLE IF EXISTS user_messenger_accounts;

DROP INDEX IF EXISTS idx_users_telegram_id_unique;

ALTER TABLE users
    ALTER COLUMN telegram_id SET NOT NULL;

ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_pkey;

ALTER TABLE users
    ADD CONSTRAINT users_pkey PRIMARY KEY (telegram_id);

ALTER TABLE users
    ALTER COLUMN id DROP DEFAULT;

ALTER TABLE users
    DROP COLUMN IF EXISTS id;

DROP SEQUENCE IF EXISTS users_id_seq;
