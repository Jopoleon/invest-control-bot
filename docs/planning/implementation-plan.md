# Implementation Planning Entrypoint

This file is the structured planning entrypoint for current work. The historical
root roadmap remains in `IMPLEMENTATION_PLAN.md`; use it for project history and
older decisions. Use this file for the current planning shape and links to the
active docs.

## Current Priorities

1. Keep the Telegram product flow stable.
2. Validate and harden recurring/autopay on real-money short periods.
3. Continue messenger-neutral refactor without cloning Telegram business logic.
4. Keep PostgreSQL and provider callbacks as production source-of-truth paths.
5. Improve operator/admin workflows with tests and documentation.
6. Keep AI/Codex session context current through `docs/context`.

## Production Host Migration: `airnet-server`

Status on 2026-08-12: cutover is complete. Production runs on `airnet-server`,
PostgreSQL 18 and `https://investcontrol.org`; the old writer is
inactive/disabled. Final dump/restore, checksum and full source/target manifest
comparison passed, and the target retained `Europe/Moscow`.

After a short explicit post-cutover pause, the owner resumed the new backend. It
is active/enabled as the only writer/callback handler; the legacy backend stays
inactive/disabled. Its canonical layout and systemd runtime are
`/home/ubuntu/apps/invest-control-bot` and `ubuntu:ubuntu`. Never start the
legacy writer at the same time.

The old hostname remains a compatibility proxy for existing Cloudflare Worker
and Robokassa URLs. MAX was switched to the new direct webhook. The migration
is now in the 24–72 hour observation/rollback-retention phase; never start the
old and new backend simultaneously.

The detailed inventory, readiness gates, DB rehearsal, cutover, observation,
and rollback plan is in `docs/ops/airnet-server-migration.md`.
The DNS-provider and new-domain variants are in
`docs/ops/domain-dns-cutover.md`; an actual domain-name change additionally
requires Nginx/TLS and Robokassa/Telegram/MAX URL changes.

The old exact `/mcp` path remains local to the separate `ad-check-bot` stack.
It is explicitly outside this migration; `investcontrol.org/mcp` returns 404.

Remaining migration follow-ups:

- update Cloudflare Worker origin after Wrangler re-authentication;
- update and verify Robokassa callback/redirect URLs;
- monitor callbacks/recurring and retain rollback state for 24–72 hours;
- establish durable encrypted off-host PostgreSQL backups;
- retire the old compatibility host only after those gates pass.

## Recurring And Autopay Hardening

Current state:

- Robokassa recurring self-integration is implemented.
- Explicit opt-in and separate recurring consent history exist.
- Short-period timing policy is centralized in `internal/app/periodpolicy`.
- Rebill request/response metadata and stale pending rebills are observable.

Next safe steps:

- Repeat production short-period live-money smoke tests after recurring changes.
- Cross-check scheduler logs, Robokassa callbacks, audit events, payment rows,
  and subscription rows before changing timing.
- Keep pending rebills distinct from successful renewals.
- Update `docs/payments/robokassa-recurring.md` after each meaningful production
  recurring milestone.

Checks:

- `GOCACHE=/tmp/go-build go test ./...`
- focused tests under `internal/app/periodpolicy`,
  `internal/app/recurring`, `internal/app/subscriptions`, and payment callback
  tests.

## Messenger-Neutral Refactor

Current state:

- `internal/messenger` holds neutral event/outbound contracts.
- Telegram and MAX are real transports over shared business logic.
- User identity is moving through `users.id` and `user_messenger_accounts`.
- `domain.Payment` and `domain.Subscription` no longer expose `TelegramID`.

Next safe steps:

- Keep resolving linked accounts through one place where practical.
- Avoid a repo-wide Telegram ID purge.
- Wire any new infrastructure into at least one production path immediately.
- Keep admin actions messenger-neutral unless a transport-specific operation is
  unavoidable.

Checks:

- `internal/bot` tests for onboarding/menu/payment paths.
- `internal/admin` tests for user detail, churn, delivery, and action flows.
- store tests for identity and linked-account behavior.

## Telegram Stability Guardrails

- Telegram remains the baseline product flow.
- Do not change Telegram behavior while adding MAX parity unless the task
  explicitly requires it.
- Telegram-specific chat operations, invite links, and member removal must stay
  observable and auditable.
- Bot rights in paid Telegram chats matter operationally: invite link creation
  and member removal can fail even when subscription logic is correct.
- Implemented link boundary: public/legacy Telegram inputs are canonicalized to
  `t.me`; Telegram Web routes are import-only; private numeric IDs never become
  bare `t.me/c` links; payment, bot and public-page fallbacks suppress unsafe
  Web URLs and public pages suppress static private invites. Admin binding also
  rejects a catalog chat that conflicts with the connector destination.
- Next UX step: replace manual chat-ID discovery with a signed
  `KeyboardButtonRequestChat` / `chat_shared` binding flow while preserving the
  current `my_chat_member` catalog and dropdown.
- Production rollout completed on 2026-08-19 as revision `5a0de97`; the
  remaining operational step is validated catalog binding/backfill for legacy
  private connectors.

## MAX Boundaries

- MAX is a second transport, not a second business flow.
- `POST /max/webhook` is the current supported runtime path.
- MAX chat/channel links may not deep-link as reliably as Telegram; bot deeplinks
  are currently the safer return path.
- Keep MAX-specific quirks in adapter or UI-return logic, not in core payment or
  subscription behavior.

## Identity Cleanup

Direction:

- Internal identity: `users.id`.
- External identities: `user_messenger_accounts`.
- Payment/subscription ownership: `user_id`.

Risk:

- Mixed-mode compatibility paths can hide Telegram-first assumptions. Remove
  them gradually only after production paths are covered.

## PostgreSQL And Migrations

- PostgreSQL is the production storage path.
- Add schema changes through migrations.
- Prefer additive migrations for risky transitions.
- Do not reintroduce legacy connector period columns.
- Keep fresh bootstrap and existing-database upgrade paths conceptually separate.

## Testing Strategy

Priority order:

1. `internal/bot`
2. `internal/app`
3. `internal/store/postgres`
4. `internal/payment`
5. `internal/store/memory` only when it supports workflow coverage

Default command:

```bash
GOCACHE=/tmp/go-build go test ./...
```

Use sqlmock for Postgres store methods and scenario tests for non-trivial
business branches. Avoid brittle order-only assertions unless order is part of
the contract.

## Documentation Discipline

- Update `docs/context/progress.md` when current status or next tasks change.
- Append to the latest session log after meaningful work.
- Update `docs/context/project-context.md` only when a durable fact changes.
- Update recurring docs immediately when recurring behavior changes.
- Update architecture/context docs when messenger identity or transport
  boundaries change.

## Active Reference Docs

- `docs/context/project-context.md`
- `docs/context/progress.md`
- `docs/payments/flow-ru.md`
- `docs/payments/robokassa-recurring.md`
- `docs/architecture/connector-period-model.md`
- `docs/architecture/max-decomposition.md`
- `docs/architecture/app-refactor.md`
- `docs/architecture/refactoring-and-tests.md`
- `docs/backlog/todo.md`
