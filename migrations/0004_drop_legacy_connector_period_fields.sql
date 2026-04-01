-- +migrate Up

ALTER TABLE connectors
    DROP COLUMN IF EXISTS test_period_seconds,
    DROP COLUMN IF EXISTS period_days;

-- +migrate Down

ALTER TABLE connectors
    ADD COLUMN IF NOT EXISTS period_days INT NOT NULL DEFAULT 30,
    ADD COLUMN IF NOT EXISTS test_period_seconds INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN connectors.period_days IS 'Legacy billing period in days retained only for downgrade compatibility.';
COMMENT ON COLUMN connectors.test_period_seconds IS 'Legacy short-lived connector period override retained only for downgrade compatibility.';
