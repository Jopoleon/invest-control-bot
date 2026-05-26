# Session Log 01 - Current

## 2026-05-09 - Documentation Memory Scaffold

### Goal

Create a `tokomak`-style project memory layer for `invest-control-bot` so future
Codex sessions have a durable context entrypoint, current progress checkpoint,
session-log discipline, and concise zone overviews.

### Files Read

- `AGENTS.md`
- `README.md`
- `IMPLEMENTATION_PLAN.md`
- `docs/README.md`
- `docs/architecture/connector-period-model.md`
- `docs/architecture/app-refactor.md`
- `docs/architecture/refactoring-and-tests.md`
- `docs/architecture/max-decomposition.md`
- `docs/payments/flow-ru.md`
- `docs/payments/robokassa-recurring.md`
- `docs/backlog/todo.md`
- repository layout under `internal/` and `migrations/`

### Actions Performed

- Added `docs/context/project-context.md` for durable facts.
- Added `docs/context/progress.md` for current status and next tasks.
- Added `docs/context/session-log.md` as the session-log index.
- Added this first chronological session log.
- Added `docs/planning/implementation-plan.md` as the structured planning
  entrypoint.
- Updated `docs/README.md` to include the context/planning layer.
- Updated `AGENTS.md` to point future sessions at the new context files and
  session-log discipline.
- Added concise `OVERVIEW.md` files for selected high-risk zones.

### Main Findings

- `AGENTS.md` was already strong and already captured most production
  invariants. It needed integration with the new context/session-log structure,
  not a rewrite.
- Existing docs already cover recurring, connector periods, MAX, app refactor,
  ops, and backlog. The missing piece was a stable documentation memory layout
  for future sessions.
- The repo is production-sensitive: recurring, callbacks, identity, migrations,
  and messenger delivery need high caution.

### Decisions

- Use `docs/context/project-context.md` for durable facts and avoid turning it
  into a changelog.
- Use `docs/context/progress.md` for active status, risks, and next tasks.
- Keep session logs chronological and focused on both actions and learning.
- Create only a small set of high-value `OVERVIEW.md` files instead of adding
  one to every directory.

### Unverified / Needs Follow-Up

- The worktree had pre-existing user code changes when this session started.
  They were not reviewed as part of this documentation pass.
- Production recurring status should be rechecked through current logs/audit/DB
  before changing recurring policy or removing short-period TODOs.
- Some roadmap details in root `IMPLEMENTATION_PLAN.md` are historical; future
  sessions should prefer the new context/progress/planning files for current
  entrypoint context while preserving root history.

### Verification

- Documentation-only pass. Go tests were not required unless code changes were
  touched.

## 2026-05-11 - Admin Telegram Link Actions Timeout

### Goal

Investigate production reports that Telegram chat links sent from admin user
actions return `504 Gateway Time-out`, and make the admin actions fail faster
with clearer labels instead of hanging until nginx closes the request.

### Files Read

- `internal/admin/user_actions.go`
- `internal/admin/localization.go`
- `internal/admin/user_detail_test.go`
- production `journalctl` logs for `invest-control-bot`
- production nginx logs

### Actions Performed

- Checked production service logs around the reported incident window.
- Verified that admin actions were reaching the backend, then failing with 500
  after long messenger/API waits and `context canceled` audit writes.
- Confirmed the service itself was up and Telegram webhook traffic was still
  arriving.
- Added a bounded timeout for synchronous admin messenger/rebill actions.
- Clarified confusing action labels:
  `Отправить ссылку на оплату`, `Отправить ссылку в Telegram-чат`,
  `Повторить автосписание`.

### Main Findings

- The user-visible `504 Gateway Time-out` was caused by admin HTTP actions
  waiting too long on outbound Telegram/relay calls.
- Some Telegram API calls through the Cloudflare Worker relay can hit the relay
  timeout while other calls, such as startup ping/webhook traffic, still work.
- The old button labels were ambiguous enough that operators could confuse
  payment-link delivery, chat-invite delivery, and manual rebill retry.

### Verification

- `GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test ./internal/admin`
- `GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test ./...`

## 2026-05-11 - Telegram Relay Error Visibility

### Goal

Explain why an admin Telegram chat-link send failed with
`error response from telegram for method sendMessage, 0` after the nginx 504
timeout fix, and improve observability for the next reproduction.

### Actions Performed

- Checked production audit and DB records for user `15` / connector `28`.
- Confirmed that a fresh one-time Telegram invite link was created and saved,
  but delivery failed on the subsequent `sendMessage` call.
- Updated the Cloudflare Worker relay to return Telegram-shaped Bot API errors
  with `error_code` and `description` so the Go Telegram SDK does not collapse
  relay failures into `method sendMessage, 0`.
- Added Telegram client warning logs and error wrapping around `sendMessage`
  and `createChatInviteLink`.

### Main Findings

- The invite-link generation path worked: the new link was persisted in
  `telegram_invite_links`.
- The immediate failure was message delivery to the Telegram user, not link
  generation or connector binding.
- The exact upstream reason was lost because the Worker returned custom error
  JSON instead of Telegram-compatible error JSON for Bot API routes.

### Verification

- `node --check telegram-bot-relay/index.js`
- `GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test ./internal/telegram ./internal/admin`
- `GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test ./...`

## 2026-05-26 - Robokassa Error 29 After Receipt Fiscalization

### Goal

Investigate persistent Robokassa checkout error `29` after fiscal receipt
support was added and after shop passwords #1/#2 were regenerated.

### Actions Performed

- Checked production env shape and recent payment rows.
- Verified that production uses `ROBOKASSA_IS_TEST_MODE=false`.
- Compared generated checkout signatures with Robokassa documentation.
- Checked that password #3 is not used by the current checkout/recurring flow.
- Changed checkout and rebill request generation so the `Receipt` request
  parameter is set to the URL-encoded receipt value, matching Robokassa payment
  examples and signature rules.
- Added regression tests for double-encoded `Receipt` in checkout URLs and
  recurring rebill form bodies.
- Added incident note:
  `docs/archive/incidents/robokassa-error-29-receipt-2026-05-26.md`.

### Main Findings

- The failure was linked to fiscalization request encoding, not password #3.
- Before the fix, the signature used encoded `Receipt`, but the request
  parameter stored raw JSON and was encoded only by the transport layer.
- Robokassa expects `Receipt` to be encoded before it is placed into the request;
  the transport then encodes that encoded value again.
- Operator confirmed that payment works after deployment of the encoding fix.

### Verification

- `GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test ./internal/payment ./internal/config ./internal/app ./...`
