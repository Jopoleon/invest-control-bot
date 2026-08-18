# Project Context

This document stores durable project facts for future engineering and Codex sessions.
It is not a changelog. Put temporary status in `docs/context/progress.md` and
chronological session notes in `docs/context/session-log/`.

## Purpose

`invest-control-bot` is a production-oriented Go backend for paid access to
messenger channels. It currently combines:

- Telegram bot onboarding and payment flow;
- MAX support as a second messenger transport;
- server-rendered admin panel;
- PostgreSQL persistence;
- Robokassa payments and recurring/autopay;
- public compliance pages for recurring checkout and cancel;
- background jobs for rebills, reminders, expiry, and access revocation.

The main product risk is not feature count. It is preserving correct payment,
subscription, and messenger identity behavior while the codebase moves from a
Telegram-first implementation toward a messenger-neutral core.

## Current Architecture

The repository is organized around stable runtime zones:

- `cmd/server` starts the long-lived HTTP backend.
- `api` and `pkg/vercelapp` contain Vercel/serverless entrypoints.
- `internal/app` owns HTTP routes, payment callbacks, public recurring pages,
  recurring jobs, lifecycle jobs, store opening, and application wiring.
- `internal/app/payments`, `internal/app/recurring`, and
  `internal/app/subscriptions` are the newer app-layer service packages split
  out of the former large `internal/app` surface.
- `internal/admin` is the server-rendered operational admin UI.
- `internal/bot` contains user-facing onboarding, registration, menu,
  subscription, and payment flows after transport updates are mapped inward.
- `internal/messenger` defines transport-neutral inbound/outbound models.
- `internal/telegram` and `internal/max` are transport-specific clients and
  adapters.
- `internal/payment` contains payment provider implementations, including
  Robokassa.
- `internal/store/postgres` is the production persistence path.
- `internal/store/memory` is for local/dev/test scenarios only.
- `internal/domain` contains durable domain models and policy helpers.
- `migrations` contains the PostgreSQL schema chain.

## Source Of Truth

- PostgreSQL 18 on `airnet-server` is the production source of truth.
- Provider callbacks are the source of truth for payment success.
- Robokassa recurring result callbacks are the source of truth for rebill
  success.
- Runtime logs and audit events are part of the operational source of truth for
  recurring incidents and access-delivery failures.
- `migrations/0001_init.sql` is the canonical fresh bootstrap schema; later
  migrations matter for existing databases.

Changing an already-applied historical migration does not upgrade an existing
database. Existing databases need a new migration version.

## Production Operations

- Canonical public origin: `https://investcontrol.org`.
- Canonical SSH alias: `airnet-server`; the application and PostgreSQL listen
  only on loopback behind Nginx.
- The `invest-control-bot` backend is active/enabled on `airnet-server`; it is
  the only application writer and callback handler. The old backend remains
  inactive/disabled.
- The application layout is `/home/ubuntu/apps/invest-control-bot`; systemd and
  every file/directory in that tree use Linux user/group `ubuntu:ubuntu`.
  The former Linux account `investcontrol` was removed; `investcontrol_app`
  remains only a PostgreSQL role.
- The previous `investcontrol-server` backend is inactive/disabled and its
  PostgreSQL 14 database is a rollback snapshot, not a writer.
- The previous `инвестконтроль.рф` hostname temporarily remains on the old
  Nginx as a compatibility proxy for provider callbacks and legacy clients.
- MAX uses the new direct webhook URL. Telegram still uses the Cloudflare
  Worker public webhook, and Robokassa callbacks may continue using the old
  hostname until their provider settings are changed.
- Never start both old and new backends: recurring/lifecycle work runs inside
  the process and both databases would diverge.

## Payment And Subscription Invariants

- Payment success is confirmed only by provider callback handling, not by
  browser redirects or success pages.
- Recurring rebill success is confirmed only by provider result callback.
- A pending rebill must never be treated as a successful renewal.
- Stale pending rebills must remain visible in logs and audit events.
- Subscription extension must use the existing rule from code:
  extend from `max(now, current period end)` where applicable.
- Access activation and revocation must stay tied to subscription state, not to
  optimistic payment-page navigation.

## Robokassa Recurring Invariants

- Recurring is explicit opt-in only.
- Recurring consent must not be pre-checked.
- Recurring consent history is separate from offer/privacy acceptance.
- Disabling autopay affects the specific subscription, not all subscriptions
  globally.
- Re-enabling autopay without a new payment is allowed only when the existing
  subscription has a recurring-capable parent payment.
- For short-period live-money smoke tests, callback visibility is the main
  operational risk. Scheduler eligibility alone is not proof of recurring
  success.

## Public Recurring Pages

- `/subscribe/{start_payload}` is a compliance/entry page. It must not become a
  hidden retrofit onto arbitrary historical payments.
- `/unsubscribe/{token}` must disable autopay for the specific subscription
  context and must not silently disable unrelated subscriptions.
- Public pages are part of payment/compliance flow, so user-facing copy, legal
  links, and provider behavior need to stay aligned.

## Messenger And Identity Direction

The long-term identity model is user-first:

- internal user identity is `users.id`;
- external messenger identity lives in `user_messenger_accounts`;
- Telegram remains supported but is no longer the canonical identity model;
- `domain.Payment` and `domain.Subscription` must not regain `TelegramID`;
- app-level notifications and audit helpers should resolve delivery targets
  from linked accounts by `user_id`.

Business logic should stay messenger-neutral. Telegram and MAX adapters should
map transport DTOs into internal messenger events and sender contracts. MAX must
not become a parallel copy of Telegram business logic.

Transport-specific behavior is allowed only where the transport actually
requires it, such as Telegram invite links, chat member removal, MAX deeplink
fallbacks, or provider/client quirks.

## Telegram Access-Link Boundary

- A Bot API `chat_id` is transport identity, not a user-facing URL.
- Public Telegram destinations use canonical `https://t.me/<username>` links.
- Private paid destinations use fresh bot-created `https://t.me/+...` invites.
- `web.telegram.org/{a,k,z}/#-100...` is only an import hint for a chat ID and
  must never appear in subscriber buttons or public payment/recurring pages.
- `t.me/c/<channel>/<message_id>` is a message link; the project must not
  synthesize bare `t.me/c/<channel>` links from numeric IDs.
- A private chat can be a valid connector destination without a public URL when
  a Bot API chat reference is available. Invite creation and later removal
  still depend on the bot's administrator rights.

## Connector Period Model

The canonical connector period model is:

- `period_mode`
  - `duration`
  - `calendar_months`
  - `fixed_deadline`
- `period_seconds`
- `period_months`
- `fixed_ends_at`

Rules:

- `duration` and `calendar_months` may be recurring, subject to provider/product
  rules.
- `fixed_deadline` is intentionally non-recurring.
- Short live-money smoke periods are ordinary `duration` connectors with small
  `period_seconds`.
- Legacy `period_days` and `test_period_seconds` must stay dead.

## Sensitive Areas

Treat these areas as high-risk:

- Robokassa callback verification and rebill handling;
- recurring scheduler timing, especially short-period policy;
- subscription lifecycle expiry/revoke coordination;
- Telegram invite-link creation and chat-member removal;
- user identity resolution across Telegram/MAX;
- PostgreSQL migrations and schema compatibility;
- public compliance pages and legal document availability.

Non-trivial changes in these areas need tests and documentation updates.

## Documentation Entry Points

- `AGENTS.md` is the agent contract and operational ruleset.
- `docs/context/progress.md` stores current state and next tasks.
- `docs/context/session-log.md` indexes chronological session notes.
- `docs/planning/implementation-plan.md` is the structured planning entrypoint.
- `docs/payments/flow-ru.md` explains payment/autopay behavior in product terms.
- `docs/payments/robokassa-recurring.md` stores Robokassa recurring requirements
  and checklist.
- `docs/architecture/connector-period-model.md` stores the canonical period
  model.
- `docs/architecture/max-decomposition.md` stores the messenger-neutral/MAX
  architecture direction.

## Needs Verification

- The current worktree may contain user-made code changes around messenger chat
  status and chat users. Future sessions should inspect `git status --short`
  before assuming docs fully describe those in-flight changes.
- Recurring production status changes quickly. Verify live-money short-period
  conclusions against current logs, audit events, and database state before
  changing recurring policy.
