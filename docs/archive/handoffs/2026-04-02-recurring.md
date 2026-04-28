# Handoff 2026-04-02

## Session Goal

Primary focus of this session was to stabilize and debug short-period recurring/autopay on real money, while keeping the codebase messenger-neutral and production-oriented.

The main reason for this work was live smoke testing with very short recurring periods like `180s` / `240s` and very small amounts.

## Current Bottom-Line Status

The backend recurring scheduler path is no longer the primary suspect.
From the latest production evidence:
- first payment succeeds
- recurring consent is saved
- subscription is created with `auto_pay_enabled = true`
- scheduler creates a pending rebill payment
- audit event `rebill_requested` is emitted
- but the rebill payment remains `pending`
- no provider callback arrives for the child rebill invoice
- subscription eventually expires normally

Current diagnosis:
- scheduler/orchestration is working far better than before
- the current most likely issue is Robokassa recurring execution/callback visibility, not the local rebill window calculation alone

## Key Production Diagnosis Captured In This Session

From the latest live production run:

- connector:
  - `connector_id = 2`
  - `period_mode = duration`
  - `period_seconds = 180`
  - `price_rub = 3`
- initial payment:
  - `payment_id = 1`
  - `status = paid`
  - `token = 684838511542728717`
  - `auto_pay_enabled = true`
- recurring consent exists for the same user and connector
- active subscription was created correctly and lasted exactly 3 minutes
- scheduler created child rebill payment:
  - `payment_id = 2`
  - `status = pending`
  - `provider_payment_id = rebill_parent:684838511542728717`
  - `token = 9033958394186460868`
  - `parent_payment_id = 1`
- audit contained:
  - `rebill_requested`
- audit did not contain:
  - successful callback/result for the child invoice
- final outcome:
  - original subscription expired
  - no extension happened

Conclusion:
- the system did attempt recurring
- provider-side rebill completion/callback remains the main unresolved area

## Major Code Changes Completed Before This Handoff

### 1. Canonical connector period model implemented

Legacy period fields were removed from runtime model.
Canonical connector period fields now are:
- `period_mode`
- `period_seconds`
- `period_months`
- `fixed_ends_at`

Supported period modes:
- `duration`
- `calendar_months`
- `fixed_deadline`

Important rule:
- `fixed_deadline` is intentionally non-recurring

Relevant files:
- `internal/domain/models.go`
- `migrations/0001_init.sql`
- `migrations/0004_drop_legacy_connector_period_fields.sql`
- admin connector creation flow and templates

### 2. Short-period recurring timing was extracted into dedicated policy package

A dedicated package was introduced:
- `internal/app/periodpolicy`

Files:
- `internal/app/periodpolicy/doc.go`
- `internal/app/periodpolicy/policy.go`
- `internal/app/periodpolicy/policy_test.go`

Purpose:
- centralize recurring timing semantics for short duration connectors
- stop leaking old day-based recurring assumptions into short smoke-test periods

Current behavior:
- short-duration rebill timing is based on actual duration
- pre-expiry reminder and expiry-notice behavior is suppressed for short periods
- short expiry grace exists to avoid immediate false terminal behavior while callback lag is still plausible

There is an intentional `TODO:` in this area to keep validating live-money behavior.

### 3. Recurring scheduler/lifecycle behavior was tightened

Relevant files:
- `internal/app/recurring/service.go`
- `internal/app/subscriptions/service.go`
- `internal/app/subscription_lifecycle.go`
- `internal/app/recurring_rebill.go`

Important changes:
- short-period recurring no longer uses the old `72h/48h/24h` mental model
- old scattered special-case logic was consolidated
- if a replacement active subscription already exists, the older row no longer emits false `expired` behavior
- if a short-period subscription has a pending rebill and is still within grace, expiry is deferred instead of immediately revoking/marking terminal too aggressively

### 4. Recurring observability was significantly improved

This was the last major focus before handoff.

#### Robokassa rebill logging
Added detailed logging around `CreateRebill(...)` in:
- `internal/payment/robokassa.go`

Now logs include:
- `invoice_id`
- `previous_invoice_id`
- `amount_rub`
- `is_test`
- target URL
- response HTTP status code
- raw response body

Expected log lines:
- `robokassa rebill request`
- `robokassa rebill response`

#### Scheduler decision logging for short periods
Added explicit decision logging in:
- `internal/app/subscription_lifecycle.go`

Expected log line:
- `short-period rebill scheduler decision`

Logged fields include:
- `subscription_id`
- `user_id`
- `connector_id`
- `remaining`
- `target_attempt`
- `failed_attempts`
- `reason`
- `trigger`

#### Stale pending rebill detection
Added warning + audit for stale pending rebills in:
- `internal/app/recurring/service.go`
- `internal/domain/audit_actions.go`

New audit action:
- `rebill_pending_stale`

Expected log line:
- `stale pending rebill without callback`

This was adjusted so stale pending rebills are still visible even after the subscription is already outside the active rebill window.
Also guarded so already-paid child payments do not generate false stale diagnostics.

### 5. Startup migration logging added

File:
- `internal/app/store_open.go`

Expected startup log lines:
- `applying database migrations`
- `database migrations applied`
- `database migrations skipped`

Purpose:
- make DB bootstrap state obvious in production logs

### 6. Bot payment-link text fixed to reflect real provider mode

Files:
- `internal/payment/service.go`
- `internal/payment/mock.go`
- `internal/payment/robokassa.go`
- `internal/bot/messages.go`
- `internal/bot/payment.go`

Previously the bot always said Robokassa was in test mode.
Now message text follows real provider mode:
- prod mode: no `test mode` wording
- mock/test mode: explicit `test mode` wording

### 7. Deploy script simplified

File:
- `scripts/deploy_vps.sh`

Current behavior:
- default mode is `simple`
- default deploy overwrites the live binary in `current/`
- release-style deploy is opt-in via `DEPLOY_LAYOUT=releases`
- service status/log tail is opt-in via env flags

This change was made because versioned release layout was adding unnecessary friction for this project.

### 8. Admin/UI cleanup largely completed

Admin presentation is now much less Telegram-first across:
- users
- user detail
- billing
- churn
- events
- connectors table
- exports
- help text

This happened across several files under:
- `internal/admin/`
- templates
- exports
- view models

MAX admin delivery and payment-link delivery were also implemented earlier in the session history.

## Documentation Updated In This Session Window

Most important docs touched or aligned during this phase:
- `AGENTS.md`
- `IMPLEMENTATION_PLAN.md`
- `README.md`
- `docs/archive/incidents/prod-recurring-2026-04-01.md`
- `docs/architecture/connector-period-model.md`

`AGENTS.md` was rewritten to reflect the current real state instead of historical assumptions.
It now explicitly calls out:
- clean-session expectations
- canonical period model
- recurring observability signals
- current top priorities

## Testing State

Core verification command used repeatedly:
```bash
GOCACHE=/tmp/go-build go test ./...
```

At the end of the latest recurring observability work, this command was green.

Newer tests added around this broad work included coverage for:
- period policy semantics
- short-period recurring decisions
- stale pending rebill detection
- short-period lifecycle behavior
- admin MAX delivery and messenger-neutral rendering

## Production Database / Migration Notes Learned During This Session

Important operational lesson captured here:
- changing the contents of an already-applied historical migration does not upgrade an existing database
- `sql-migrate` tracks version files, not rewritten file contents

This became relevant because:
- `0001_init.sql` was rewritten to represent the clean baseline
- existing DBs that had already applied the old `0001` did not automatically receive the new schema

For disposable environments, the practical solution used was:
- drop runtime tables / reset schema
- restart service
- let startup migrations recreate the clean schema

Observed startup logs after reset:
- `applying database migrations`
- `database migrations applied applied=4`

## Codex / Session / MCP Notes

### Fresh sessions
To get repo-local Codex config correctly, start a new session from the repository root:
```bash
cd ~/Work/src/github.com/Jopoleon/invest-control-bot
codex
```

Do not rely on `codex resume` from another repository if you need repo-local MCP configuration to apply.

### Repo-local MCP for prod Postgres through SSH tunnel
Added helper script:
- `scripts/prod_postgres_tunnel.sh`

Added setup doc:
- `docs/ops/prod-postgres-mcp.md`

Intended flow:
1. start SSH tunnel to prod Postgres locally
2. add repo-local MCP entry to `.codex/config.toml`
3. start a fresh Codex session from this repo root

Note:
- this session environment could not safely rewrite `.codex/config.toml`
- the needed prod MCP snippet is documented in `docs/ops/prod-postgres-mcp.md`

### Current session limitations that motivated handoff
This session could edit repository files but could not write to `.git` internals for `git commit`.
That is why commit text had to be provided manually instead of committing inside this session.

## Most Important Open Technical Work After This Handoff

### P0: finish real-money recurring diagnosis with new logs
This is the next concrete task.

Recommended procedure:
1. deploy current code
2. run live smoke with short-period recurring connector (`180s` or `240s`)
3. watch for these log lines:
   - `short-period rebill scheduler decision`
   - `robokassa rebill request`
   - `robokassa rebill response`
   - `stale pending rebill without callback`
4. compare with DB state:
   - parent payment
   - child pending rebill payment
   - callback/result arrival or absence

The key question now is:
- does Robokassa actually execute the child recurring charge and callback to us, or does it stop before that?

### P1: if rebill request returns 200 but no callback follows
Then likely next actions are:
- inspect provider-side recurring records in Robokassa cabinet
- capture whether child invoice exists provider-side
- confirm ResultURL behavior for rebill path
- add even more provider-side correlation logging if necessary

### P1: keep short-period policy intentionally marked as not-final
Do not remove the validation `TODO:` from short-period recurring code yet.
We still need repeated real-money confirmation.

## Files Most Worth Reopening First In A New Session

If resuming work on recurring diagnosis, open these first:
- `AGENTS.md`
- `internal/app/periodpolicy/policy.go`
- `internal/app/recurring/service.go`
- `internal/app/subscription_lifecycle.go`
- `internal/app/subscriptions/service.go`
- `internal/payment/robokassa.go`
- `internal/app/store_open.go`
- `docs/archive/incidents/prod-recurring-2026-04-01.md`
- `docs/ops/prod-postgres-mcp.md`

## Suggested First Prompt For The New Session

Use something close to this:

```text
Read AGENTS.md and docs/archive/handoffs/2026-04-02-recurring.md first.
Primary task: continue diagnosing real-money short-period recurring.
We already know scheduler creates pending rebill payments; the main suspected issue is Robokassa rebill completion/callback visibility.
Use the new recurring logs and, if needed, the repo-local production Postgres MCP through SSH tunnel.
Do not redo the broad refactor work; continue from the current recurring diagnosis state.
```
