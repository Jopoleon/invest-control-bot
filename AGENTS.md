# AGENTS.md

## Project

`invest-control-bot` is a Go backend for paid access to messenger channels with:
- Telegram bot onboarding and payment flow
- server-rendered admin panel
- PostgreSQL storage
- Robokassa payment and recurring/autopay support
- public compliance pages for recurring checkout/cancel
- an ongoing messenger-neutral refactor with MAX already integrated as a second transport

The codebase is production-oriented and under active refactor. The current product priority is short-period recurring validation on real money without regressing the main Telegram flow.

## Primary Goals

1. Keep the current Telegram product stable.
2. Preserve and debug recurring/autopay logic before expanding product scope.
3. Continue the messenger-neutral refactor without cloning Telegram business logic into MAX-specific forks.
4. Add tests together with every non-trivial refactor.

## Fresh Session Bootstrap

For a clean Codex session in this repository:
- start a new session from the repository root, not by resuming a session that was created in another repo
- repo-local MCP config lives in `.codex/config.toml`
- the expected local MCP server is `wsl_local_postgres`
- the main verification command is:
```bash
GOCACHE=/tmp/go-build go test ./...
```

Useful first checks in a clean session:
```bash
git status --short
GOCACHE=/tmp/go-build go test ./...
```

Useful recurring/ops logs to recognize quickly:
- `applying database migrations`
- `database migrations applied`
- `short-period rebill scheduler decision`
- `robokassa rebill request`
- `robokassa rebill response`
- `stale pending rebill without callback`

## Current Architectural State

### Stable layers
- `internal/app`: HTTP server, payment callbacks, recurring lifecycle jobs, public pages, store bootstrap
- `internal/admin`: admin panel and sessions
- `internal/payment`: mock + Robokassa providers
- `internal/store/postgres`: primary persistence
- `internal/store/memory`: local/dev support and test scenarios

### Messenger-neutral foundation
- `internal/messenger` contains transport-neutral outbound and inbound models
- `internal/telegram/client.go` implements the sender contract
- `internal/max` is integrated as a real transport, not a parallel business flow
- `internal/bot/update_router.go` localizes Telegram/MAX update mapping into internal messenger events
- internal user identity is `users.id`
- external messenger identity lives in `user_messenger_accounts`
- `domain.Payment` and `domain.Subscription` no longer expose `TelegramID`
- app-level notifications and audit helpers resolve messenger targets through linked accounts by `user_id`

### Connector period model
The canonical connector period model is already implemented.
Do not reintroduce legacy duration fields.

Current connector fields:
- `period_mode`
  - `duration`
  - `calendar_months`
  - `fixed_deadline`
- `period_seconds`
- `period_months`
- `fixed_ends_at`

Implications:
- short smoke-test periods are ordinary `duration` connectors now
- `fixed_deadline` is intentionally non-recurring
- old `period_days` / `test_period_seconds` should stay dead

### Recurring observability state
Short-period recurring has dedicated timing policy and observability.
Relevant code:
- `internal/app/periodpolicy`
- `internal/app/recurring/service.go`
- `internal/app/subscriptions/service.go`
- `internal/app/subscription_lifecycle.go`
- `internal/payment/robokassa.go`

What exists now:
- short-duration rebill timing is centralized in `internal/app/periodpolicy`
- scheduler logs a decision line for each short-period subscription it evaluates
- Robokassa rebill calls log request and response metadata
- stale pending rebills are surfaced through warning logs and audit events
- short-duration expiry/reminder behavior is intentionally different from long-lived production periods

This area is still considered sensitive and must be treated carefully.

## Repository Map

- `cmd/server` - app entrypoint
- `api` - Vercel entrypoints
- `internal/app` - HTTP app, payment handlers, recurring pages, schedulers, store bootstrap
- `internal/admin` - admin panel
- `internal/bot` - user-facing messenger flow
- `internal/messenger` - messenger-neutral transport contracts
- `internal/payment` - payment providers
- `internal/store/postgres` - primary store
- `internal/store/memory` - dev/test store
- `internal/domain` - domain model
- `migrations` - DB schema
- `docs` - detailed project docs
- `.codex/config.toml` - repo-local Codex MCP config for clean sessions

## Critical Business Invariants

### Payments / subscriptions
- payment success is confirmed only by provider callback handling, not redirect pages
- recurring rebill success is confirmed only by provider result callback
- subscription extension must use the existing business rule from current code: extend from `max(now, current period end)` where applicable
- a pending rebill must never be treated as a successful renewal
- stale pending rebills must be visible in logs/audit rather than silently ignored

### Robokassa recurring
- recurring is explicit opt-in only
- no pre-checked recurring consent
- recurring consent history is stored separately from offer/privacy acceptance
- disabling autopay affects the specific subscription, not all subscriptions globally
- re-enabling autopay is allowed without new payment only when the existing subscription has a recurring-capable parent payment
- for short-period smoke tests, provider callback visibility is currently the main risk area, not scheduler eligibility alone

### Public recurring pages
- `/subscribe/{start_payload}` is a compliance/entry page, not a magic retrofit onto arbitrary historical payments
- `/unsubscribe/{token}` must work for the specific subscription and must not silently disable unrelated subscriptions

### Messenger architecture
- do not fork Telegram logic into a second MAX-only business flow
- keep business logic messenger-neutral
- adapters should map transport DTOs into internal events and sender contracts
- admin delivery/actions should stay messenger-neutral unless a transport-specific action is truly unavoidable

## Identity Refactor Rules

Current direction:
- internal user identity is `users.id`
- messenger identity is modeled separately
- Telegram remains supported but is not the long-term canonical identity
- `users` runtime/profile model no longer stores messenger metadata inline
- linked account resolution should happen in one place and then flow through `user_id`
- mixed-mode compatibility still exists in some read paths and public/provider flows

When changing code:
1. Prefer store methods that resolve users through messenger identity.
2. Keep existing Telegram behavior unchanged unless the bug/feature explicitly requires changing it.
3. Do not attempt a repo-wide telegram-id purge in one pass.
4. If a step introduces new infrastructure, wire at least one production path to it immediately.

## Admin/UI State

Admin is now mostly messenger-neutral across:
- users
- user detail
- billing
- churn
- events
- connectors table
- exports
- help text

Do not reintroduce Telegram-first labels or mandatory Telegram-only assumptions in new admin work.
Transport-specific actions may still exist, but they should be clearly marked as such.

## Testing Policy

Refactoring and tests go together.

Priority for tests:
1. `internal/bot`
2. `internal/app`
3. `internal/store/postgres`
4. `internal/payment`
5. `internal/store/memory` only when it adds workflow coverage

Preferred test style:
- unit tests around use-case logic
- sqlmock tests for postgres store methods
- scenario tests for non-trivial business branches
- avoid brittle order-only assertions unless order is a real contract

Main command:
```bash
GOCACHE=/tmp/go-build go test ./...
```

Default `go test ./...` should stay green too.

## Comment / TODO Policy

Comments are required where intent is not obvious:
- transport boundaries
- recurring/autopay rules
- identity resolution logic
- non-obvious invariants
- package-level `doc.go` comments for the main `internal/*` packages

Do not add noise comments for trivial assignments.
When logic changes, comments must change with it.

If a place remains intentionally incomplete, risky, environment-specific, or still under discussion, leave a searchable `TODO:` comment nearby instead of keeping the assumption implicit.
This is especially important for:
- payment callbacks
- recurring timing
- short-period smoke-test behavior
- subscription expiry/rebill coordination
- messenger delivery behavior

## DB / Migration Rules

- PostgreSQL is the source of truth
- do not introduce schema changes without a migration
- prefer additive migrations for risky transitions
- keep compatibility windows only when they are actually needed
- do not resurrect legacy connector period columns or legacy schema assumptions

Current baseline:
- `migrations/0001_init.sql` is the canonical bootstrap schema
- later migrations continue from that clean baseline
- `internal/app/store_open.go` is responsible for store opening and startup migration logging

Important operational note:
- changing the contents of an already-applied historical migration does not upgrade an existing database
- for existing databases, only new migration versions matter

## Deploy / Ops Notes

Current deploy script behavior:
- `scripts/deploy_vps.sh` defaults to simple deploy mode
- default behavior overwrites the live binary in `current/`
- release-style deploy is opt-in through `DEPLOY_LAYOUT=releases`
- service status/log printing is opt-in through flags

Current payment-mode behavior:
- bot payment-link text reflects the real provider mode now
- production Robokassa should explicitly use `ROBOKASSA_IS_TEST_MODE=false`

## Docs To Keep In Sync

When you make meaningful product or architecture changes, update the relevant docs:
- `README.md`
- `IMPLEMENTATION_PLAN.md`
- `docs/PROD_BUGFIX_TRACK_2026-04-01.md`
- `docs/MAX_IMPLEMENTATION_PLAN.md`
- `docs/APP_REFACTOR_PLAN.md`
- `docs/REFACTORING_AND_TEST_PLAN.md`
- `docs/CONNECTOR_PERIOD_MODEL_PLAN.md`
- `docs/PAYMENTS_FLOW_RU.md`
- `docs/robokassa-recurring-checklist.md`

If a change affects recurring behavior, update recurring docs immediately.

## Known Practical Constraints

- `vendor/` exists and can drift; if `go test ./...` breaks because of vendor mode, resync with:
```bash
GOCACHE=/tmp/go-build go mod vendor
```
- Vercel deployment excludes `vendor/` via `.vercelignore`
- recurring logic is sensitive to document availability:
  - offer
  - privacy
  - user agreement
- a clean local/prod recurring smoke test is not valid unless the legal docs path is configured correctly

## Current Top Priorities

1. Validate and debug real-money short-period recurring end-to-end.
2. Improve provider/callback visibility around rebill attempts before making product-level assumptions.
3. Continue the remaining identity cleanup incrementally.
4. Only after that, keep pushing MAX parity on top of the messenger-neutral core.

## Known Follow-Ups To Preserve

- short-period connectors are intentional and currently used for live recurring smoke tests
- short-period timing policy is centralized in `internal/app/periodpolicy`
- keep explicit `TODO:` markers near short-period recurring code until repeated live-money validation confirms the behavior
- stale pending rebills should remain observable through both logs and audit events
- startup migration logs are intentionally explicit and should stay that way

## Useful Files

- `internal/app/periodpolicy/policy.go`
- `internal/app/recurring/service.go`
- `internal/app/subscriptions/service.go`
- `internal/app/subscription_lifecycle.go`
- `internal/app/store_open.go`
- `internal/payment/robokassa.go`
- `internal/bot/update_router.go`
- `internal/bot/user_identity.go`
- `internal/messenger/types.go`
- `internal/messenger/events.go`
- `internal/store/postgres/users.go`
- `migrations/0001_init.sql`
- `.codex/config.toml`
- `IMPLEMENTATION_PLAN.md`
- `docs/PROD_BUGFIX_TRACK_2026-04-01.md`
- `docs/CONNECTOR_PERIOD_MODEL_PLAN.md`
- `docs/robokassa-recurring-checklist.md`
