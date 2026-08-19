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
- Telegram access now separates private Bot API chat identity from public
  navigation: official links are canonicalized to `t.me`, Telegram Web URLs
  are import-only, and private numeric chat IDs no longer become invalid bare
  `t.me/c/<id>` links.

## Active Work

- Real-money short-period recurring validation remains the top operational
  priority.
- Production migration to `airnet-server`, PostgreSQL 18 and
  `https://investcontrol.org` completed on 2026-08-12. After a requested pause,
  the owner explicitly resumed the new backend: it is active/enabled and is the
  only writer/callback handler; the old service is inactive/disabled.
- The new service layout was consolidated under
  `/home/ubuntu/apps/invest-control-bot`; systemd runs as `ubuntu:ubuntu` and
  every application-tree file/directory has that owner/group. Future deploys
  default to the same layout and must not introduce another Linux service user.
  The obsolete `investcontrol` Linux account/home was removed after confirming
  that it had no processes or service data.
- The final PostgreSQL custom dump checksum and source/target manifests matched.
  Production retained `Europe/Moscow`, all 9 migrations, 18 users, 144 payments
  and 115 subscriptions at the cutover boundary.
- The previous production hostname remains a temporary HTTPS compatibility
  proxy to the new origin. Its exact `/mcp` route stays on the unrelated old
  stack; the new hostname intentionally returns 404 for `/mcp`.
- MAX has only the new-domain webhook subscription. Telegram remains healthy
  through its Worker (`pending=0`, no last error); a synthetic empty update
  passed Worker→legacy proxy→new backend with HTTP 200. The Worker secret could
  not be updated because local Wrangler authentication is unavailable, so the
  old hostname proxy currently preserves its origin route.
- Messenger-neutral refactor is ongoing, especially around delivery, identity
  resolution, and transport-specific access actions.
- Admin UI is being refined for operational clarity around connectors,
  subscriptions, churn, billing, and access recovery.
- App-layer code is being split into clearer services under
  `internal/app/payments`, `internal/app/recurring`, and
  `internal/app/subscriptions`.
- Documentation is being reorganized to keep AI/Codex session context durable.
- The Telegram deep-link hardening was deployed to `airnet-server` as revision
  `5a0de97` on 2026-08-19. Post-deploy health, Telegram/MAX startup checks,
  database access and all six active legacy-Web connector checkout pages passed;
  the service has zero restarts and no new WARN/ERROR. Existing production
  connectors still need a controlled catalog binding/backfill pass. Public
  payment/recurring pages expose only public Telegram usernames; static/private
  invites remain bot-delivered after confirmed access, and admin chat binding
  rejects destination mismatches.

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

- Monitor the resumed new service, Nginx, payment callbacks and recurring/audit
  state for 24–72 hours before removing rollback assets. Do not start the
  legacy writer.
- Re-authenticate Wrangler and set
  `TELEGRAM_WEBHOOK_ORIGIN_URL=https://investcontrol.org/telegram/webhook`, then
  verify a real Telegram update reaches the new origin directly.
- Change and verify Robokassa ResultURL/SuccessURL/FailURL to the new hostname;
  until then the legacy-host compatibility proxy remains mandatory.
- Establish a durable encrypted off-host PostgreSQL backup. The final 0600 dump
  is temporarily retained outside the repo, target has a protected archive,
  and the old frozen DB remains available, but this is not the final backup
  policy.
- Recheck public DNS after Airnet's four-hour TTL expires; Quad9 still returned
  the provider's former IP during the immediate post-cutover check.
- After provider URL changes and the observation window, decide the retirement
  date for the old Nginx/PostgreSQL host. `/mcp` can remain independently or be
  retired with its own project.
- Deploy current recurring/access fixes and repeat short-period live-money smoke
  tests.
- Deploy the Telegram link hardening, bind every private paid chat through the
  discovered-chat dropdown, and verify new payment plus "Моя подписка" paths
  issue fresh `t.me/+...` links. Do not bulk-rewrite unverified chat IDs.
- Design the later `KeyboardButtonRequestChat` / `chat_shared` connector flow;
  keep the current `my_chat_member` catalog and dropdown as the safe manual
  path.
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

- When can the old hostname compatibility proxy and frozen PostgreSQL 14
  rollback source be retired?

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
