-- Production test-data cleanup draft.
--
-- Generated for the inventory in:
-- docs/ops/prod-test-data-cleanup-2026-05-18.md
--
-- IMPORTANT:
-- - Review before execution.
-- - This script is intentionally destructive but ends with ROLLBACK by default.
-- - To actually apply it, inspect the RETURNING/count output and replace the
--   final ROLLBACK with COMMIT in the execution copy.
-- - Connector 15 is intentionally excluded. Do not add it to target_connectors.

BEGIN;

-- Keep the transaction from hanging on unexpected locks.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

-- ---------------------------------------------------------------------------
-- 1. Safety preview
-- ---------------------------------------------------------------------------

WITH
target_accounts(messenger_kind, messenger_user_id) AS (
  VALUES
    ('telegram', '264704572'),
    ('max', '193465776')
),
target_users AS (
  SELECT DISTINCT u.id
  FROM users u
  JOIN user_messenger_accounts a ON a.user_id = u.id
  JOIN target_accounts ta
    ON ta.messenger_kind = a.messenger_kind
   AND ta.messenger_user_id = a.messenger_user_id
),
target_connectors(id) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(11),(12),(13)
)
SELECT
  'PREVIEW_TARGET_USERS' AS section,
  u.id,
  u.full_name,
  u.phone,
  u.email,
  string_agg(
    a.messenger_kind || ':' || a.messenger_user_id || ':' || COALESCE(NULLIF(a.username, ''), '∅'),
    ', ' ORDER BY a.messenger_kind, a.messenger_user_id
  ) AS accounts
FROM users u
JOIN target_users tu ON tu.id = u.id
LEFT JOIN user_messenger_accounts a ON a.user_id = u.id
GROUP BY u.id, u.full_name, u.phone, u.email
ORDER BY u.id;

WITH target_connectors(id) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(11),(12),(13)
)
SELECT
  'PREVIEW_TARGET_CONNECTORS' AS section,
  c.id,
  c.name,
  c.start_payload,
  c.is_active,
  c.price_rub,
  c.period_mode,
  c.period_seconds,
  c.period_months
FROM connectors c
JOIN target_connectors tc ON tc.id = c.id
ORDER BY c.id;

-- Abort if connector 15 accidentally gets into scope.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM (VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(11),(12),(13)) AS target_connectors(id)
    WHERE id = 15
  ) THEN
    RAISE EXCEPTION 'Safety stop: connector 15 must not be deleted';
  END IF;
END $$;

-- Abort if any target connector currently grants active access. Historical
-- active-but-expired rows are acceptable; current/future access is not.
DO $$
DECLARE
  active_count BIGINT;
BEGIN
  SELECT COUNT(*)
  INTO active_count
  FROM subscriptions s
  JOIN (VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(11),(12),(13)) AS target_connectors(id)
    ON target_connectors.id = s.connector_id
  WHERE s.status = 'active'
    AND s.ends_at > NOW();

  IF active_count > 0 THEN
    RAISE EXCEPTION 'Safety stop: % current/future active subscriptions exist on target connectors', active_count;
  END IF;
END $$;

-- ---------------------------------------------------------------------------
-- 2. Delete dependent operational/history rows.
-- ---------------------------------------------------------------------------

WITH
target_accounts(messenger_kind, messenger_user_id) AS (
  VALUES
    ('telegram', '264704572'),
    ('max', '193465776')
),
target_users AS (
  SELECT DISTINCT u.id
  FROM users u
  JOIN user_messenger_accounts a ON a.user_id = u.id
  JOIN target_accounts ta
    ON ta.messenger_kind = a.messenger_kind
   AND ta.messenger_user_id = a.messenger_user_id
),
target_connectors(id) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(11),(12),(13)
),
target_subscriptions AS (
  SELECT s.id
  FROM subscriptions s
  WHERE s.user_id IN (SELECT id FROM target_users)
     OR s.connector_id IN (SELECT id FROM target_connectors)
),
deleted AS (
  DELETE FROM audit_events e
  WHERE e.actor_user_id IN (SELECT id FROM target_users)
     OR e.target_user_id IN (SELECT id FROM target_users)
     OR e.connector_id IN (SELECT id FROM target_connectors)
  RETURNING e.id
)
SELECT 'DELETED audit_events' AS action, COUNT(*) AS rows FROM deleted;

WITH
target_accounts(messenger_kind, messenger_user_id) AS (
  VALUES
    ('telegram', '264704572'),
    ('max', '193465776')
),
target_users AS (
  SELECT DISTINCT u.id
  FROM users u
  JOIN user_messenger_accounts a ON a.user_id = u.id
  JOIN target_accounts ta
    ON ta.messenger_kind = a.messenger_kind
   AND ta.messenger_user_id = a.messenger_user_id
),
target_connectors(id) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(11),(12),(13)
),
target_subscriptions AS (
  SELECT s.id
  FROM subscriptions s
  WHERE s.user_id IN (SELECT id FROM target_users)
     OR s.connector_id IN (SELECT id FROM target_connectors)
),
deleted AS (
  DELETE FROM messenger_chat_user_checks c
  WHERE c.user_id IN (SELECT id FROM target_users)
     OR c.connector_id IN (SELECT id FROM target_connectors)
     OR c.subscription_id IN (SELECT id FROM target_subscriptions)
  RETURNING c.id
)
SELECT 'DELETED messenger_chat_user_checks' AS action, COUNT(*) AS rows FROM deleted;

WITH
target_accounts(messenger_kind, messenger_user_id) AS (
  VALUES
    ('telegram', '264704572'),
    ('max', '193465776')
),
target_users AS (
  SELECT DISTINCT u.id
  FROM users u
  JOIN user_messenger_accounts a ON a.user_id = u.id
  JOIN target_accounts ta
    ON ta.messenger_kind = a.messenger_kind
   AND ta.messenger_user_id = a.messenger_user_id
),
deleted AS (
  DELETE FROM messenger_chat_users c
  WHERE c.user_id IN (SELECT id FROM target_users)
  RETURNING c.messenger_kind, c.chat_ref, c.messenger_user_id
)
SELECT 'DELETED messenger_chat_users' AS action, COUNT(*) AS rows FROM deleted;

WITH
target_accounts(messenger_kind, messenger_user_id) AS (
  VALUES
    ('telegram', '264704572'),
    ('max', '193465776')
),
target_connectors(id) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(11),(12),(13)
),
deleted AS (
  DELETE FROM registration_states r
  WHERE EXISTS (
      SELECT 1
      FROM target_accounts ta
      WHERE ta.messenger_kind = r.messenger_kind
        AND ta.messenger_user_id = r.messenger_user_id
    )
     OR r.connector_id IN (SELECT id FROM target_connectors)
  RETURNING r.messenger_kind, r.messenger_user_id
)
SELECT 'DELETED registration_states' AS action, COUNT(*) AS rows FROM deleted;

WITH
target_accounts(messenger_kind, messenger_user_id) AS (
  VALUES
    ('telegram', '264704572'),
    ('max', '193465776')
),
target_users AS (
  SELECT DISTINCT u.id
  FROM users u
  JOIN user_messenger_accounts a ON a.user_id = u.id
  JOIN target_accounts ta
    ON ta.messenger_kind = a.messenger_kind
   AND ta.messenger_user_id = a.messenger_user_id
),
target_connectors(id) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(11),(12),(13)
),
deleted AS (
  DELETE FROM user_consents c
  WHERE c.user_id IN (SELECT id FROM target_users)
     OR c.connector_id IN (SELECT id FROM target_connectors)
  RETURNING c.user_id, c.connector_id
)
SELECT 'DELETED user_consents' AS action, COUNT(*) AS rows FROM deleted;

WITH
target_accounts(messenger_kind, messenger_user_id) AS (
  VALUES
    ('telegram', '264704572'),
    ('max', '193465776')
),
target_users AS (
  SELECT DISTINCT u.id
  FROM users u
  JOIN user_messenger_accounts a ON a.user_id = u.id
  JOIN target_accounts ta
    ON ta.messenger_kind = a.messenger_kind
   AND ta.messenger_user_id = a.messenger_user_id
),
target_connectors(id) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(11),(12),(13)
),
deleted AS (
  DELETE FROM recurring_consents c
  WHERE c.user_id IN (SELECT id FROM target_users)
     OR c.connector_id IN (SELECT id FROM target_connectors)
  RETURNING c.id
)
SELECT 'DELETED recurring_consents' AS action, COUNT(*) AS rows FROM deleted;

-- ---------------------------------------------------------------------------
-- 3. Delete invite links, subscriptions, payments.
-- ---------------------------------------------------------------------------

WITH
target_accounts(messenger_kind, messenger_user_id) AS (
  VALUES
    ('telegram', '264704572'),
    ('max', '193465776')
),
target_users AS (
  SELECT DISTINCT u.id
  FROM users u
  JOIN user_messenger_accounts a ON a.user_id = u.id
  JOIN target_accounts ta
    ON ta.messenger_kind = a.messenger_kind
   AND ta.messenger_user_id = a.messenger_user_id
),
target_connectors(id) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(11),(12),(13)
),
target_subscriptions AS (
  SELECT s.id
  FROM subscriptions s
  WHERE s.user_id IN (SELECT id FROM target_users)
     OR s.connector_id IN (SELECT id FROM target_connectors)
),
deleted AS (
  DELETE FROM telegram_invite_links l
  WHERE l.user_id IN (SELECT id FROM target_users)
     OR l.connector_id IN (SELECT id FROM target_connectors)
     OR l.subscription_id IN (SELECT id FROM target_subscriptions)
  RETURNING l.id
)
SELECT 'DELETED telegram_invite_links' AS action, COUNT(*) AS rows FROM deleted;

WITH
target_accounts(messenger_kind, messenger_user_id) AS (
  VALUES
    ('telegram', '264704572'),
    ('max', '193465776')
),
target_users AS (
  SELECT DISTINCT u.id
  FROM users u
  JOIN user_messenger_accounts a ON a.user_id = u.id
  JOIN target_accounts ta
    ON ta.messenger_kind = a.messenger_kind
   AND ta.messenger_user_id = a.messenger_user_id
),
target_connectors(id) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(11),(12),(13)
),
deleted AS (
  DELETE FROM subscriptions s
  WHERE s.user_id IN (SELECT id FROM target_users)
     OR s.connector_id IN (SELECT id FROM target_connectors)
  RETURNING s.id
)
SELECT 'DELETED subscriptions' AS action, COUNT(*) AS rows FROM deleted;

WITH
target_accounts(messenger_kind, messenger_user_id) AS (
  VALUES
    ('telegram', '264704572'),
    ('max', '193465776')
),
target_users AS (
  SELECT DISTINCT u.id
  FROM users u
  JOIN user_messenger_accounts a ON a.user_id = u.id
  JOIN target_accounts ta
    ON ta.messenger_kind = a.messenger_kind
   AND ta.messenger_user_id = a.messenger_user_id
),
target_connectors(id) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(11),(12),(13)
),
deleted AS (
  DELETE FROM payments p
  WHERE p.user_id IN (SELECT id FROM target_users)
     OR p.connector_id IN (SELECT id FROM target_connectors)
  RETURNING p.id
)
SELECT 'DELETED payments' AS action, COUNT(*) AS rows FROM deleted;

-- ---------------------------------------------------------------------------
-- 4. Delete target connectors and target users.
-- ---------------------------------------------------------------------------

WITH target_connectors(id) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(11),(12),(13)
),
deleted AS (
  DELETE FROM connectors c
  WHERE c.id IN (SELECT id FROM target_connectors)
  RETURNING c.id, c.name
)
SELECT 'DELETED connectors' AS action, COUNT(*) AS rows FROM deleted;

WITH
target_accounts(messenger_kind, messenger_user_id) AS (
  VALUES
    ('telegram', '264704572'),
    ('max', '193465776')
),
target_users AS (
  SELECT DISTINCT u.id
  FROM users u
  JOIN user_messenger_accounts a ON a.user_id = u.id
  JOIN target_accounts ta
    ON ta.messenger_kind = a.messenger_kind
   AND ta.messenger_user_id = a.messenger_user_id
),
deleted AS (
  DELETE FROM users u
  WHERE u.id IN (SELECT id FROM target_users)
  RETURNING u.id
)
SELECT 'DELETED users' AS action, COUNT(*) AS rows FROM deleted;

-- ---------------------------------------------------------------------------
-- 5. Final verification inside the same transaction.
-- ---------------------------------------------------------------------------

WITH
target_accounts(messenger_kind, messenger_user_id) AS (
  VALUES
    ('telegram', '264704572'),
    ('max', '193465776')
),
target_connectors(id) AS (
  VALUES (1),(2),(3),(4),(5),(6),(7),(8),(9),(11),(12),(13)
)
SELECT 'VERIFY_REMAINING_TARGET_ACCOUNTS' AS check_name, COUNT(*) AS rows
FROM user_messenger_accounts a
JOIN target_accounts ta
  ON ta.messenger_kind = a.messenger_kind
 AND ta.messenger_user_id = a.messenger_user_id
UNION ALL
SELECT 'VERIFY_REMAINING_TARGET_CONNECTORS', COUNT(*)
FROM connectors c
JOIN target_connectors tc ON tc.id = c.id
UNION ALL
SELECT 'VERIFY_CONNECTOR_15_EXISTS', COUNT(*)
FROM connectors c
WHERE c.id = 15
UNION ALL
SELECT 'VERIFY_REMAINING_TARGET_PAYMENTS', COUNT(*)
FROM payments p
WHERE p.connector_id IN (SELECT id FROM target_connectors)
UNION ALL
SELECT 'VERIFY_REMAINING_TARGET_SUBSCRIPTIONS', COUNT(*)
FROM subscriptions s
WHERE s.connector_id IN (SELECT id FROM target_connectors)
ORDER BY check_name;

-- Default is safe dry-run. Replace with COMMIT only in the reviewed execution copy.
ROLLBACK;
