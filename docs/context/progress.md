# Current Progress

This file is the current checkpoint for future sessions. It should answer:
where the project is now, what is sensitive, and what concrete work should come
next.

## Status Summary

The project has a working Telegram product flow, MAX integrated as a second
transport, PostgreSQL-backed persistence, Robokassa payments, recurring/autopay
support, public recurring pages, and an operational admin panel.

The current engineering focus is stabilization, not broad scope expansion:

1. Keep Telegram stable.
2. Continue validating recurring/autopay on real money.
3. Advance messenger-neutral identity and delivery cleanup incrementally.
4. Improve admin/operator workflows without regressing payment/subscription
   invariants.
5. Keep docs and tests synchronized with meaningful changes.

## Stable Or Mostly Stable

- Telegram onboarding/payment flow exists and remains the baseline product path.
- MAX is integrated through the shared backend and current supported runtime is
  webhook/server mode.
- PostgreSQL is the main storage path and `internal/store/postgres` is the
  priority for meaningful persistence coverage.
- `internal/store/memory` supports local/dev/test flows but is not a product
  priority.
- `domain.Payment` and `domain.Subscription` are `user_id`-first and no longer
  expose `TelegramID`.
- Connector periods use the canonical `period_mode` model.
- Short-period recurring timing is centralized in `internal/app/periodpolicy`.
- Robokassa rebill request/response metadata and stale pending rebills have
  explicit observability in logs/audit.
- Admin screens are mostly messenger-neutral in presentation.

## Active Work

- Real-money short-period recurring validation remains the top operational
  priority.
- Messenger-neutral refactor is ongoing, especially around delivery, identity
  resolution, and transport-specific access actions.
- Admin UI is being refined for operational clarity around connectors,
  subscriptions, churn, billing, and access recovery.
- App-layer code is being split into clearer services under
  `internal/app/payments`, `internal/app/recurring`, and
  `internal/app/subscriptions`.
- Documentation is being reorganized to keep AI/Codex session context durable.

## Sensitive Areas

- Robokassa callback and recurring result handling.
- Short-period recurring windows and expiry grace behavior.
- Subscription expiry, renewal, revoke, and invite-link lifecycle ordering.
- Telegram chat/member operations and bot rights.
- MAX deeplink and channel-link behavior.
- Public recurring checkout/cancel pages and legal document availability.
- PostgreSQL migrations for existing databases.
- Any compatibility bridge that still reads historical Telegram-centric state.

## Next Concrete Tasks

- Deploy current recurring/access fixes and repeat short-period live-money smoke
  tests.
- Cross-check recurring incidents with journald, audit events, payment rows, and
  subscription rows before drawing timing conclusions.
- Update recurring docs after the next production confirmation if callback
  visibility is no longer the main unresolved risk.
- Continue identity cleanup in small production-wired steps rather than a
  repo-wide Telegram ID purge.
- Strengthen tests around payment pages, recurring pages, lifecycle jobs, and
  bot subscription/payment branches.
- Improve connector/operator admin screens while keeping business behavior
  unchanged.

## Open Questions

- Which mixed-mode compatibility paths can be removed after the next identity
  cleanup milestone?
- Does recurring cancel token need to encode `messenger_kind` explicitly to avoid
  Telegram-first fallback assumptions in mixed-mode cases?
- Should access issuance/revocation get a dedicated persistent membership model,
  or should old `chat_memberships` roadmap text be formally retired?
- Does MAX provide a stable documented way to deep-link into specific chats, or
  should the bot deeplink remain the primary CTA indefinitely?
- After repeated live-money validation, should short-period recurring TODOs be
  removed or narrowed?

## Verification Notes

Main verification command:

```bash
GOCACHE=/tmp/go-build go test ./...
```

For recurring incidents, verify both logs and database state. Useful log patterns:

- `short-period rebill scheduler decision`
- `robokassa rebill request`
- `robokassa rebill response`
- `stale pending rebill without callback`
- `payment marked as paid`
- `/payment/result`

For documentation-only changes, Go tests are optional, but final reports should
say explicitly when tests were skipped because no code changed.

## Current Worktree Note

At the start of the documentation pass that created this context layer, the
worktree already contained user code changes around messenger chat status/users
and transport clients. Treat those as user-owned unless a future task explicitly
asks to modify them.
