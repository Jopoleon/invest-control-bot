-- +migrate Up

-- Historical no-op.
-- Short connector periods are now modeled by canonical connectors.period_mode = duration
-- together with connectors.period_seconds. This migration name is retained so
-- existing schema_migrations rows remain valid and fresh bootstrap stays linear.

-- +migrate Down

-- Historical no-op.
