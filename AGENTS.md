# AGENTS.md

## Project

`invest-control-bot` is a Go backend for paid access to Telegram channels/chats with:
- Telegram bot onboarding and payment flow
- server-rendered admin panel
- PostgreSQL storage
- Robokassa recurring/autopay support
- public compliance pages for recurring checkout/cancel
- ongoing refactor toward multi-messenger support, with MAX as the next channel

The current codebase is production-oriented, but still in active architectural refactor.

## Primary Goals

1. Keep the current Telegram product stable.
2. Preserve and expand recurring/autopay logic.
3. Continue refactoring toward messenger-neutral architecture without duplicating Telegram logic.
4. Increase unit-test coverage while refactoring.

## Current Architectural State

### Stable layers
- `internal/app`: HTTP server, payment callbacks, recurring lifecycle jobs, public pages
- `internal/admin`: admin panel and sessions
- `internal/payment`: mock + Robokassa
- `internal/store/postgres`: primary persistence
- `internal/store/memory`: local/dev support, lower priority for exhaustive tests

### Messenger refactor status
- `internal/messenger` already contains transport-neutral outbound and inbound models
- `internal/telegram/client.go` implements the sender contract
- `internal/bot` no longer depends on Telegram DTOs internally; Telegram update mapping is localized in `internal/bot/update_router.go`
- internal `user_id` foundation and messenger identities have been introduced in domain/store/migrations
- `internal/bot` already started using `GetOrCreateUserByMessenger` in registration flow

### Still not finished
- many business records still use `telegram_id` directly:
  - payments
  - subscriptions
  - consents
  - recurring pages
  - admin user detail / exports
- do not try to replace all `telegram_id` references in one pass
- the correct path is incremental: identity resolution first, downstream foreign-key migration later
 - `audit_events` has already been generalized into actor/target records and no longer assumes Telegram-only user context

## Repository Map

- `cmd/server` - app entrypoint
- `api` - Vercel entrypoints
- `internal/app` - HTTP app, payment handlers, recurring pages, schedulers
- `internal/admin` - admin panel
- `internal/bot` - user-facing messenger flow
- `internal/messenger` - messenger-neutral transport contracts
- `internal/payment` - payment providers
- `internal/store/postgres` - primary store
- `internal/store/memory` - dev/test store
- `internal/domain` - domain model
- `migrations` - DB schema
- `docs` - detailed project docs

## Critical Business Invariants

### Payments / subscriptions
- payment success is confirmed only by provider callback handling, not by redirect pages
- recurring rebill success is confirmed only by provider result callback
- subscription extension must use the existing business rule from current code: extend from `max(now, current period end)` where applicable

### Robokassa recurring
- recurring is explicit opt-in only
- no pre-checked recurring consent
- recurring consent history is stored separately from offer/privacy acceptance
- disabling autopay must affect the specific subscription, not all subscriptions globally
- re-enabling autopay is allowed without new payment only when the existing subscription has a recurring-capable parent payment

### Public recurring pages
- `/subscribe/{start_payload}` is a compliance/entry page, not a magic “retrofit recurring onto any old payment” switch
- `/unsubscribe/{token}` must work for конкретная подписка logic and must not silently disable unrelated subscriptions

### Messenger architecture
- do not fork Telegram logic into a second MAX-only business flow
- keep business logic messenger-neutral
- adapters should map transport DTOs into internal events and sender contracts

## User Identity Refactor Rules

Current direction:
- internal user identity is `users.id`
- external messenger identity is modeled separately
- Telegram remains supported through `telegram_id`, but it is no longer the long-term canonical identity
- `user_settings.auto_pay_enabled` has been removed from the runtime model; recurring state is derived from subscriptions and explicit checkout choice
- `users` runtime/profile model no longer carries `telegram_id` / `telegram_username`; messenger metadata lives in `user_messenger_accounts`
- postgres `payments` / `subscriptions` writes now align with the clean baseline schema; legacy `telegram_id` is treated as a derived runtime projection resolved through linked Telegram identity
- `GetLatestSubscriptionByUserConnector` and `DisableAutoPayForActiveSubscriptions` are now `user_id`-first store APIs; bot/app runtime paths should prefer resolved internal users over Telegram IDs for ownership and recurring-state changes
- app-level notification/audit helpers now resolve messenger targets through linked accounts by `user_id`; temporary preferred messenger ids may still be passed from mixed-mode records, but the helper contract is no longer built around `legacyExternalID`

When changing code:
1. Prefer using store methods that resolve users through messenger identity.
2. Keep existing Telegram behavior unchanged.
3. Avoid mass schema rewrites of payments/subscriptions until identity resolution is fully centralized.
4. If a step only introduces new infrastructure, wire at least one real production path to it, so it does not remain dead code.

## Testing Policy

Refactoring and tests go together.

Priority for tests:
1. `internal/bot`
2. `internal/app`
3. `internal/store/postgres`
4. `internal/payment`
5. `internal/store/memory` only when useful for workflow coverage, not as a primary target

Preferred test style:
- unit tests around use-case logic
- sqlmock tests for postgres store methods
- avoid brittle order-only assertions when behavior does not guarantee strict order
- prefer scenario tests over tiny parser-only tests when the business branch is non-trivial

Useful command:
```bash
GOCACHE=/tmp/go-build go test ./...
```

Default `go test ./...` should also stay green.

## Comment Policy

Comments are required where intent is not obvious:
- transport boundaries
- tricky recurring/autopay rules
- identity resolution logic
- non-obvious invariants

Do not add noise comments for trivial assignments.
When logic changes, comments must be updated in the same change.

## DB / Migration Rules

- PostgreSQL is the source of truth
- do not introduce schema changes without a migration
- for risky schema transitions, prefer additive migrations first
- keep backward-compatible read/write paths during migration windows whenever possible
- clean-schema pass has already been executed for the current repository state:
  - historical additive migrations were squashed into a fresh baseline
  - `migrations/0001_init.sql` now represents the current canonical bootstrap schema
  - local PostgreSQL was reset and the clean bootstrap was verified from an empty database
  - future schema changes should continue from this new baseline using additive migrations

Current important migration baseline:
- `0001_init.sql`
  - contains the current canonical schema for connectors, users, messenger accounts, payments, subscriptions, recurring data and admin sessions
  - reflects the post-refactor clean bootstrap state, not the historical step-by-step evolution

Any further migration of payments/subscriptions from `telegram_id` to `user_id` should be staged, not done in one shot.

## Docs To Keep In Sync

When you make meaningful product or architecture changes, update the relevant docs:

- `README.md` - current operational/project summary
- `IMPLEMENTATION_PLAN.md` - main working roadmap and progress log
- `docs/MAX_IMPLEMENTATION_PLAN.md` - MAX-specific track
- `docs/REFACTORING_AND_TEST_PLAN.md` - engineering backlog for tests, deduplication and safe refactors
- `docs/PAYMENTS_FLOW_RU.md` - payment/autopay explanation
- `docs/robokassa-recurring-checklist.md` - recurring operational/legal checklist

If the change affects recurring behavior, update docs immediately.

## Known Practical Constraints

- `vendor/` exists and can drift; if default `go test ./...` breaks because of vendor mode, resync with:
```bash
GOCACHE=/tmp/go-build go mod vendor
```
- Vercel deployment excludes `vendor/` via `.vercelignore`
- recurring logic is sensitive to document availability:
  - offer
  - privacy
  - user agreement

## Current Recommended Next Steps

If continuing implementation from current state, the next sensible sequence is:

1. Continue replacing direct Telegram user lookups in app/admin flows with centralized identity resolution.
2. Add tests for every such refactor step.
3. Only after that, start migrating business records toward `user_id`.
4. Then implement the MAX adapter on top of the messenger-neutral bot core.

## Useful Files

- `internal/bot/update_router.go`
- `internal/bot/user_identity.go`
- `internal/messenger/types.go`
- `internal/messenger/events.go`
- `internal/store/postgres/users.go`
- `internal/store/memory/store.go`
- `migrations/0001_init.sql`
- `IMPLEMENTATION_PLAN.md`
- `docs/MAX_IMPLEMENTATION_PLAN.md`
- `docs/REFACTORING_AND_TEST_PLAN.md`
