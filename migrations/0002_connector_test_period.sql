-- +migrate Up

-- Allow explicit short-lived connector periods for payment and recurring smoke
-- tests without replacing the default day-based production period.
ALTER TABLE connectors
    ADD COLUMN test_period_seconds INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN connectors.test_period_seconds IS 'Optional short-lived test override in seconds. Example: 900 for 15m or 90 for 90s. Value 0 keeps normal day-based period_days semantics.';
