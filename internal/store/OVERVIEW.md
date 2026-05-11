# internal/store Overview

`internal/store` defines persistence contracts and implementations.

## Responsibilities

- `store.go` defines the interfaces required by app, bot, admin, payment, and
  lifecycle flows.
- `postgres` is the production implementation.
- `memory` supports local/dev/test workflows.

## Storage Priority

PostgreSQL is the source of truth. Meaningful persistence behavior should be
validated against `internal/store/postgres` first. The memory store should be
touched only when needed for local/dev behavior or useful workflow tests.

## Invariants

- Internal user identity is `users.id`.
- Messenger identities live in `user_messenger_accounts`.
- Payment and subscription records are `user_id`-first.
- Schema changes require migrations.
- Do not reintroduce legacy connector period columns.
- Existing databases are upgraded only by new migrations, not by editing old
  already-applied migrations.

## Checks

For Postgres store changes, prefer sqlmock tests near the changed methods.

Full check:

```bash
GOCACHE=/tmp/go-build go test ./...
```

## Related Docs

- `docs/context/project-context.md`
- `docs/planning/implementation-plan.md`
- `docs/ops/migrations.md`
- `docs/architecture/connector-period-model.md`
