# Prod Test Data Cleanup Inventory - 2026-05-18

This document records a read-only production database inventory for planned
cleanup of historical test data. No delete/update SQL was executed while
collecting these facts.

## Scope

### Target users

Planned for cleanup:

| user_id | Messenger | Account | Name | Phone | Email |
|---:|---|---|---|---|---|
| 1 | Telegram | `264704572` / `@emiloserdov` | `SAFASF` | `+72438124124` | `eag@mail.com` |
| 2 | MAX | `193465776` / `@Федор Николаевич` | `Егор М` | `+7823414124124` | `egasfdasdf@mail.com` |

### Target connectors

Planned for cleanup:

| connector_id | Name | Payload | Status | Price | Period |
|---:|---|---|---|---:|---|
| 1 | `test-telega-recurent` | `in-dec144a94e4b06ff` | active | 3 ₽ | 3 min |
| 2 | `test-telega-recurent2` | `in-8b54e54ea2ec3417` | active | 3 ₽ | 3 min |
| 3 | `тест-телега3` | `in-e9466fed9ef70e7e` | disabled | 4 ₽ | 3 min |
| 4 | `тест-телега-рекурент-4` | `in-584b31751ea74ae6` | disabled | 3 ₽ | 3 min |
| 5 | `тест-3часа-подписка` | `in-3e5f3e827cb5eafb` | disabled | 2 ₽ | 3 h |
| 6 | `test-3h-recurring` | `in-515d3e55be9c246a` | active | 1 ₽ | 3 h |
| 7 | `final-test-reccuring-6h` | `in-b28c940c7e4d0220` | active | 1 ₽ | 6 h |
| 8 | `Тест Ксения` | `in-b6fe17112558d5a5` | active | 1 ₽ | 30 d |
| 9 | `Тест 2` | `in-e5f9636493e0d24a` | active | 1 ₽ | 5 min |
| 11 | `тест-отписки-6м` | `in-df425e745d68ff19` | active | 1 ₽ | 6 min |
| 12 | `test-timed-links` | `in-24086838cea7fac0` | active | 1 ₽ | 5 min |
| 13 | `test-telega-invitelink` | `in-e50376b3d4e5d912` | active | 1 ₽ | 5 min |

Explicitly excluded from cleanup:

| connector_id | Name | Reason |
|---:|---|---|
| 15 | `Ритейл групп (непубличная оферта)` | Required production connector; do not delete. |

## Read-Only Findings

### Rows linked to target users

| Table / relation | Rows |
|---|---:|
| `payments` | 59 |
| `subscriptions` | 55 |
| `user_consents` | 9 |
| `recurring_consents` | 7 |
| `user_messenger_accounts` | 2 |
| `audit_events` as actor | 94 |
| `audit_events` as target | 466 |

### Rows linked to target connectors

The counts below originally included connector `15` in the first investigation.
Before executing any cleanup SQL, rerun the counts with connector `15` removed
from the target list.

| Table / relation | Rows with original target list including connector 15 |
|---|---:|
| `payments` | 71 |
| `subscriptions` | 65 |
| `user_consents` | 15 |
| `recurring_consents` | 14 |
| `telegram_invite_links` | 2 |
| `audit_events` | 577 |

### Users affected by connector deletion

Deleting the target connectors affects more than users `1` and `2`.
This is expected for test connector cleanup, but must be confirmed before
running destructive SQL.

| user_id | Name | Account | Target connectors involved |
|---:|---|---|---|
| 1 | `SAFASF` | Telegram `264704572` / `@emiloserdov` | 1, 2, 3, 4, 5, 6 |
| 2 | `Егор М` | MAX `193465776` / `@Федор Николаевич` | 6, 7 |
| 3 | `Шевцов Федор Николаевич` | Telegram `284218410` / `@evansteo` | 6, 8; connector 15 excluded |
| 4 | `ИнвестКонтроль` | Telegram `7973916550` / `@invest_control` | 9 |
| 6 | `Дебиторка` | Telegram `8027559719` / `@debitorka_control` | 11, 12, 13 |

### Pending payments found during inventory

| payment_id | user_id | connector_id | Connector | Amount | Notes |
|---:|---:|---:|---|---:|---|
| 2 | 1 | 2 | `test-telega-recurent2` | 3 ₽ | test cleanup candidate |
| 64 | 4 | 9 | `Тест 2` | 1 ₽ | test cleanup candidate if connector 9 is confirmed disposable |
| 71 | 3 | 15 | `Ритейл групп (непубличная оферта)` | 3000 ₽ | do not touch in this cleanup because connector 15 is excluded |

## Deletion Strategy Draft

Prepared SQL draft:

- `docs/ops/prod-test-data-cleanup-2026-05-18.sql`

Do not execute as-is for permanent deletion. The script intentionally ends with
`ROLLBACK`. To apply it after review, replace the final `ROLLBACK` with
`COMMIT` in an execution copy.

Planned deletion order:

1. Define `target_users` by messenger identity, not only by numeric `users.id`.
2. Define `target_connectors` as connector IDs `1,2,3,4,5,6,7,8,9,11,12,13`.
3. Export or snapshot affected rows before deletion.
4. Delete dependent rows first where foreign keys do not cascade:
   `audit_events`, `messenger_chat_user_checks`, `messenger_chat_users`,
   `registration_states`, `user_consents`, `recurring_consents`.
5. Delete `telegram_invite_links` for target users/connectors.
6. Delete `subscriptions` and `payments` for target users/connectors in an
   order that respects `subscriptions.payment_id -> payments.id`.
7. Delete target connector rows.
8. Delete target user rows.
9. Run final verification SELECTs in the same transaction before `COMMIT`.

Important constraints:

- Do not include connector `15`.
- Do not delete user `3` globally; only rows tied to disposable test connectors
  may be candidates.
- Confirm that all active/current subscription counts remain zero for the final
  target connector list before deletion.
- Prefer `BEGIN; ... verification SELECTs ... COMMIT;` and keep a `ROLLBACK`
  variant available for dry-run review.

## Dry-Run Result

Dry-run executed against production on 2026-05-18. The script completed
successfully and ended with `ROLLBACK`; no data was changed.

Target connectors in dry-run:

- `1,2,3,4,5,6,7,8,9,11,12,13`
- connector `15` was excluded and verification confirmed it still exists.

Rows that would be deleted:

| Relation | Rows |
|---|---:|
| `audit_events` | 601 |
| `messenger_chat_user_checks` | 0 |
| `messenger_chat_users` | 0 |
| `registration_states` | 0 |
| `user_consents` | 15 |
| `recurring_consents` | 13 |
| `telegram_invite_links` | 2 |
| `subscriptions` | 65 |
| `payments` | 70 |
| `connectors` | 12 |
| `users` | 2 |

Final verification inside the rolled-back transaction:

| Check | Rows |
|---|---:|
| `VERIFY_CONNECTOR_15_EXISTS` | 1 |
| `VERIFY_REMAINING_TARGET_ACCOUNTS` | 0 |
| `VERIFY_REMAINING_TARGET_CONNECTORS` | 0 |
| `VERIFY_REMAINING_TARGET_PAYMENTS` | 0 |
| `VERIFY_REMAINING_TARGET_SUBSCRIPTIONS` | 0 |
