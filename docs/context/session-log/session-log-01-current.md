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

## 2026-08-04 - Airnet Production Migration Discovery and Plan

### Goal

Inventory the current production host, assess the new `airnet-server` host,
and create a safe migration runbook before changing runtime infrastructure.

### Actions Performed

- Verified the effective `airnet-server` SSH configuration and retried the
  key-based connection.
- Read the current deployment, systemd, migrations, app/payment, Telegram
  relay, PostgreSQL and ops documentation.
- Performed read-only inventory of `investcontrol-server`: service, runtime
  paths, Nginx, TLS, listeners, firewall, PostgreSQL, scheduled jobs, database
  metadata/counts, applied migrations, and public DNS.
- Added `docs/ops/airnet-server-migration.md` with staged bootstrap, rehearsal,
  cutover, validation, and rollback gates.

### Main Findings

- At the initial earlier check `airnet-server` accepted key-based SSH as
  `ubuntu`. On the later check it returned `No route to host` for
  `46.8.195.244:22`; no changes were attempted on the new host.
- The old production app is healthy behind Nginx, with PostgreSQL local-only.
  Its database is small enough for a logical dump/restore rehearsal, but no
  dedicated backup job was found in the basic server inventory.
- The public production domain currently resolves to the old host and has a
  valid certificate. Keeping that domain stable is essential for Robokassa,
  Telegram relay and MAX callbacks.
- The old host mixes release and simple deployment layouts: `current` is a
  release symlink while a newer binary was overwritten in place. The new host
  plan explicitly uses a physical `current/` directory with simple deploy.

### Decisions

- No production cutover or new-host provisioning is performed before network
  reachability is restored and the operator can validate the new host.
- A logical PostgreSQL migration, pre-cutover restore rehearsal, explicit
  callback checks, and delayed old-host retirement are mandatory.
- New-server clone testing must avoid active production provider and messenger
  side effects because recurring/lifecycle work is in-process.

### Verification

- `ssh -o BatchMode=yes airnet-server ...` — blocked by network routing at the
  later check.
- Read-only `ssh investcontrol-server ...` inventory — completed.
- Documentation-only change; Go tests were not run because production code,
  configuration, and migrations were unchanged.

## 2026-08-05 - Airnet Connectivity Restored and Both Hosts Rechecked

### Goal

Repeat the full migration-readiness check after `airnet-server` became
reachable again, and correct the runbook with facts from both hosts.

### Actions And Findings

- Confirmed stable key-based SSH to `ubuntu@46.8.195.244` and matched the
  authorized public-key fingerprint to `~/.ssh/airnet_server_ed25519`.
- Inventoried the clean Ubuntu 26.04 VM: 2 vCPU, 3.3 GiB RAM, 50 GiB XFS,
  PostgreSQL/Nginx/Certbot not yet installed, UFW disabled, password SSH still
  allowed, and 46 system updates pending.
- Confirmed outbound HTTPS reachability to Telegram API, Cloudflare relay,
  Robokassa and MAX.
- Confirmed `ubuntu` belongs to sudo but interactive authentication is required;
  no system preparation was attempted without it.
- Rechecked old production: service healthy at revision `2ed7bd2`, database
  counts unchanged, DNS/TLS healthy, no new payment/rebill activity and still
  no dedicated PostgreSQL backup job.
- Found that the shared production domain routes `/mcp` to an unrelated process
  on port 18081. This route must be migrated or proxied as part of DNS cutover.
- Audited the cutover plan for duplicate in-process schedulers, DB14→DB18
  restore, TLS/DNS propagation, env ownership, backup and rollback gaps; updated
  `docs/ops/airnet-server-migration.md` accordingly.

### Verification

- `GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test ./...` — passed.
- Read-only SSH inventories of `airnet-server` and `investcontrol-server` —
  passed.
- No DNS, database, provider, secret, service or firewall changes were made.

## 2026-08-05 - Airnet Host Bootstrap and Database Rehearsal

### Goal

Prepare the new VPS up to the final production cutover boundary without
starting a second production-configured runtime.

### Actions Performed

- Confirmed permanent passwordless sudo for `ubuntu`, as explicitly chosen by
  the server owner.
- Updated Ubuntu, installed kernel `7.0.0-29`, rebooted and verified SSH.
- Installed PostgreSQL 18.4, Nginx 1.28.3, Certbot 4.0 and fail2ban.
- Enabled UFW for 22/80/443 only; disabled password/root SSH and X11 forwarding.
- Created nologin service user `investcontrol`, simple deployment directories,
  persistent runtime log directory and root-only PostgreSQL credentials.
- Installed a disabled/inactive systemd unit and staged binary revision
  `2ed7bd2` without production env.
- Fixed `SKIP_RESTART=1` in `scripts/deploy_vps.sh`; the old branch accidentally
  rebuilt the default restart command. The accidental no-env start attempt
  never spawned the binary and was stopped/reset.
- Added checked-in Nginx configs. The `/mcp` route intentionally returns 503
  until its separate target is decided.
- Transferred the existing Let’s Encrypt state directly old→new over SSH and
  matched the certificate public-key fingerprint.
- Verified public HTTP/HTTPS through `46.8.195.244` with `curl --resolve` using
  a memory/mock process running as `investcontrol`, then stopped it.
- Rehearsed PostgreSQL 14→18 custom dump/restore in a disposable DB. All table
  counts, sequences, indexes and comparable constraints matched; objects were
  owned by `investcontrol_app`.
- Dropped the rehearsal DB and removed temporary dumps from local, old/new
  transfer paths after verification.

### Verification

- `GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test ./...` — passed before staging.
- `bash -n scripts/deploy_vps.sh` — passed.
- Live `SKIP_RESTART=1` scenario — service remained disabled/inactive.
- `nginx -t`, key-only SSH, UFW, fail2ban, PostgreSQL SCRAM login and local-only
  listener — passed.
- PostgreSQL rehearsal manifest — matched after excluding PostgreSQL 18's new
  type-`n` representation of 141 `NOT NULL` constraints.
- Production DNS, old database, provider settings and webhooks were unchanged.

## 2026-08-10 - Server Recheck And New-Domain DNS Audit

### Goal

Recheck both production hosts and document the correct DNS setup for a new
domain/provider without starting the production cutover.

### Actions And Findings

- Reconfirmed key-only SSH to `airnet-server`, matching key fingerprint,
  permanent passwordless sudo, hardened SSH, UFW/fail2ban, local-only
  PostgreSQL and healthy Nginx/Certbot. The application remains
  disabled/inactive; production env and database remain absent.
- Found no failed units or reboot requirement on the new host. Twenty-two
  package updates have accumulated since bootstrap and should be installed
  before the migration window.
- Rechecked old production through the actual configured SSH alias
  `investcontrol-server`; the spelling `invest_control` is not a configured
  local alias.
- Confirmed the old service is active/enabled without restarts at revision
  `2ed7bd2`; local and public health checks return 200. Production now contains
  141 payments and 115 subscriptions, two more of each than on 2026-08-05, so
  final counts/dump must be captured after stopping the old backend.
- Identified `/mcp` as the API of a separate `ad-check-bot` container stack with
  its own PostgreSQL database and token. It must be migrated as a stack or
  temporarily proxied to the old host.
- Confirmed Telegram provider webhook health through the Cloudflare Worker and
  exactly one MAX subscription on the current production hostname. The Worker
  origin secret and Robokassa dashboard callback settings remain external
  checks.
- Audited authoritative and public DNS for `инвестконтроль.рф`: apex A remains
  `192.144.13.87` with authoritative TTL 3600, `www` is a CNAME to apex, no
  AAAA/DNSSEC is configured, and the `.рф` NS delegation TTL is four days.
- Confirmed that current TLS/Nginx cover only the apex hostname, so the existing
  `www` CNAME is not by itself a working HTTPS endpoint.
- Added `docs/ops/domain-dns-cutover.md` with separate procedures for a brand
  new domain and a DNS-hosting migration, a minimal record set, and the related
  Nginx/TLS/Robokassa/Telegram/MAX changes.

### Verification

- Read-only SSH inventory of both hosts and authoritative/public DNS queries —
  passed.
- No DNS, database, provider, secret, service or firewall changes were made.
- Documentation-only update; Go tests were not run because production code was
  unchanged.

## 2026-08-10 - Final PostgreSQL Migration Preflight

### Goal

Determine whether the live PostgreSQL 14 data can be migrated safely into the
prepared PostgreSQL 18 cluster and identify the exact final-cutover boundary.

### Actions And Findings

- Performed read-only source and target PostgreSQL inventories without creating
  a dump or exposing credentials.
- Confirmed a healthy 11.5 MB source with 16 public tables, 10 aligned
  sequences, migrations `0001`–`0009`, no invalid indexes, unvalidated
  constraints, long transactions, blocking or replication.
- Recorded the current non-final manifest: 18 users, 141 payments, 115
  subscriptions and 1568 audit events.
- Confirmed a clean PostgreSQL 18.4 target: production DB absent, app role is a
  non-privileged SCRAM login, local-only listener and 41 GB free.
- Found the source/target timezone difference. The final target DB must set
  `TimeZone=Europe/Moscow` explicitly after creation.
- Confirmed that recent traffic is quiet: no payment/rebill/callback activity
  in the last 24 hours, no imminent autopay renewal, no long DB transaction and
  no active backend connection at the sample time.
- Estimated DB freeze/dump/transfer/restore/manifest verification at 2–5
  minutes.
- Determined that a final DB transfer cannot be performed separately from
  application cutover: leaving the old writer active would immediately make
  the target stale.
- Found that the deployed old unit still uses `TimeoutStopSec=15`; the tracked
  and new-server unit use 60 seconds. This must be reconciled before final stop
  or explicitly accepted as an operational risk.

### Verification And State

- Both audits were read-only. The old backend remains active and the new
  backend remains disabled/inactive.
- No production DB, dump, service, DNS, provider setting or secret was changed.
- Go tests were not run because production code was unchanged.

## 2026-08-12 - `investcontrol.org` DNS Zone Preparation

### Goal

Translate the Airnet DNS control-panel fields into the minimal safe records for
the new project domain without changing the live production hostname.

### Findings

- Confirmed public delegation of `investcontrol.org` to `dns1.airnet.uz` and
  `dns2.airnet.uz`.
- Found the Airnet default apex A record `176.96.243.100`; it must be replaced,
  not supplemented, with the new VPS public IPv4 `46.8.195.244`.
- Confirmed the default `www` CNAME to apex, MX to apex, and `mail`/`ftp`
  CNAMEs. For a web-only zone, MX, `mail` and `ftp` are unnecessary; `www`
  requires matching Nginx and TLS support.
- Confirmed no public AAAA for the new domain; none should be added until the
  VPS has a verified global IPv6 path.

### State And Verification

- DNS was queried read-only. No record, server, service or provider setting was
  changed by the agent.
- The new hostname is not yet configured in Nginx/TLS and must not be treated
  as a healthy application endpoint merely after saving its A record.
- Documentation-only update; Go tests were not run.

## 2026-08-12 - Production Cutover To Airnet And PostgreSQL 18

### Goal

Move the only production writer, PostgreSQL data and canonical public hostname
from the legacy VPS to `airnet-server`/`investcontrol.org`, while preserving
legacy provider callbacks through a compatibility bridge.

### Pre-cutover State And Authorization

- The owner explicitly authorized final migration, old-production stop and new
  production start; `/mcp` was declared outside the required migration scope.
- Authoritative Airnet DNS already returned `investcontrol.org A 46.8.195.244`
  and `www` CNAME to apex. Airnet's actual TTL is 14400 seconds; Quad9 still
  cached the former provider IP while Cloudflare/Google saw the new one.
- Old production was quiet: no active DB query/long transaction/backend
  connection and no payment/result/rebill activity in the immediate gate.
- Final pre-stop counts were 18 users, 144 payments, 115 subscriptions and 1595
  audit events.

### Preparation

- Installed the available Ubuntu updates on the target; no reboot requirement
  remained. Three GRUB updates were deferred by phased rollout.
- Added the canonical Nginx vhost for apex/`www`, with HTTP→HTTPS and
  `www`→apex redirects. New `/mcp` intentionally returns 404.
- Issued a separate Let's Encrypt certificate for apex/`www`, expiring
  2026-11-10, and passed `certbot renew --dry-run`.
- Streamed the old production env directly SSH→SSH without a local plaintext
  copy. Preserved all secrets and `APP_ENCRYPTION_KEY`; replaced DB credentials,
  public/MAX URLs and later the log path. Installed the target env as
  `root:root 0600` and validated target SCRAM login without printing values.
- Increased the old unit's effective graceful stop timeout from 15 to 60
  seconds without restarting it.

### Database Cutover And Recovery Exercises

- Stopped the old writer with systemd and verified `MainPID=0` plus zero old app
  DB sessions before every final dump.
- First restore completed, but the cross-version manifest used the PG14
  `lc_collate` GUC removed in PG18. The scripted failure path restarted the old
  writer. The manifest was changed to query `pg_database`; the already-restored
  snapshot then matched source exactly.
- First new-server startup failed before store/provider initialization because
  the inherited relative `LOG_FILE_PATH=logs/` was not writable in the deploy
  directory. The activation wrapper disabled the target and restored the old
  writer. The path was changed to the prepared `shared/logs/app.log`.
- Because the old writer had restarted, the target DB was dropped and a fresh
  final freeze/dump/restore was performed rather than reusing a potentially
  stale snapshot.
- Final custom dump SHA-256:
  `619fd73e8a6319893ecb16f2e6723656a47534064ea360091ce8fcc6982e4606`.
- PostgreSQL 18 restore ran in one transaction into a fresh UTF8/libc
  `en_US.UTF-8` DB owned by `investcontrol_app`; DB timezone was explicitly set
  to `Europe/Moscow`.
- Final source/target manifests matched exactly across all table counts,
  migrations, sequences, object owners, index health and comparable
  constraints. App-role SCRAM login passed.

### Activation And Compatibility

- Started and enabled the target service at revision `2ed7bd2`; startup applied
  zero pending migrations, verified Telegram/MAX APIs, retained the Telegram
  Worker webhook and replaced the old MAX subscription with
  `https://investcontrol.org/max/webhook`.
- New backend became healthy with `NRestarts=0`. Landing, legal pages, admin
  login, health and invalid ResultURL handling passed through public HTTPS.
- Replaced only the old Nginx `location /` with a verified HTTPS proxy to the
  target IP using SNI/Host `investcontrol.org`. The exact legacy `/mcp` route
  remains local and healthy.
- Verified old-host and new-host application responses, including matching 400
  responses for an empty invalid ResultURL request.
- Disabled the old backend at boot; it is inactive/disabled with frozen DB.
  New backend is active/enabled. Deploy and production tunnel defaults now use
  `airnet-server`.

### External Provider State

- Telegram provider webhook still targets the Cloudflare Worker, with
  `pending_update_count=0` and no last error. Outbound relay ping passed.
- A synthetic empty Telegram update with the existing webhook secret traversed
  Worker→legacy Nginx→new backend and returned 200 without a business action.
- Local Wrangler authentication was unavailable, so the Worker origin secret
  was not changed. The legacy hostname proxy preserves its current origin path.
- Robokassa dashboard URLs were not available to this session. Their old
  hostname remains functional through the compatibility proxy until manually
  changed and live-tested.

### Verification And Retention

- New host: system running, zero failed units, UFW allows only 22/80/443,
  PostgreSQL and backend listen only on loopback, Nginx config valid, no reboot
  required, no WARN/ERROR after successful activation.
- Old host: backend inactive/disabled, Nginx/PostgreSQL active, compatibility
  proxy valid.
- Stale temporary dump copies were removed. The final 0600 local dump and
  matching manifests are retained temporarily outside the repo; target archive
  and old frozen DB remain for the observation window.
- `bash -n scripts/deploy_vps.sh scripts/prod_postgres_tunnel.sh` — passed.
- `git diff --check` — passed apart from existing line-ending warnings.
- Go tests were not rerun because no Go production code changed during cutover;
  the deployed revision was the previously tested `2ed7bd2` binary.

### Follow-ups

- Observe logs, callbacks, audit and DB state for 24–72 hours.
- Re-authenticate Wrangler and point Worker origin directly at the new domain.
- Update/verify Robokassa Result/Success/Fail URLs.
- Establish durable encrypted off-host PostgreSQL backup policy.
- Retire old compatibility/rollback state only after provider and observation
  gates pass.

## 2026-08-12 - Backend Pause And Ubuntu-Only Application Ownership

### Request

Pause `invest-control-bot` on the new server and make Linux user `ubuntu` the
only deploy/runtime owner for all application files and directories.

### Changes

- Stopped and disabled `invest-control-bot` on `airnet-server`; verified
  `inactive`, `disabled`, `MainPID=0` and `NRestarts=0`. The legacy backend was
  already inactive/disabled, so no application writer or callback handler is
  currently running.
- Moved the application tree from `/home/investcontrol/apps/invest-control-bot`
  to `/home/ubuntu/apps/invest-control-bot` while the service was stopped.
- Recursively set the complete application tree to `ubuntu:ubuntu`; retained
  mode `0600` for the production env and PostgreSQL bootstrap credential.
- Updated the absolute `LOG_FILE_PATH` to the new writable `shared/logs` path
  without printing or copying secret values.
- Updated the systemd unit to run as `User=ubuntu`, `Group=ubuntu` and use only
  `/home/ubuntu/apps/...` paths. Reloaded systemd and deliberately left the unit
  disabled/inactive.
- Verified that the obsolete Linux account `investcontrol` had no processes or
  service data, removed the account, and deleted its remaining three standard
  shell profile files plus empty home. This does not affect the retained
  PostgreSQL role `investcontrol_app`.
- Updated deploy defaults, operator instructions, current context and planning
  docs to enforce the same ubuntu-only application layout for future work.

### Verification

- No file or directory under the new application tree has an owner/group other
  than `ubuntu:ubuntu`.
- The production env is readable as `ubuntu`, remains mode `0600`, and contains
  the new absolute log path.
- A protected TCP/SCRAM check sourced that env as Linux user `ubuntu` and
  reached PostgreSQL as role `investcontrol_app`; timezone remained
  `Europe/Moscow` and the users count remained 18.
- `systemd-analyze verify` passed for the installed unit; only unrelated Ubuntu
  warnings about removed XFS `CPUAccounting` options were emitted.
- The old application path no longer exists. PostgreSQL, Nginx and the VPS were
  intentionally not stopped or re-owned; the old Linux account/home no longer
  exists and `investcontrol_app` remains a database role rather than a Linux
  runtime user. The removed shell profiles contained no service data and can be
  recreated from `/etc/skel` if that obsolete account is ever recreated.
- Go tests were skipped because no Go production code changed.
- Both canonical and legacy proxied `/healthz` return the expected HTTP 502
  while the backend is deliberately paused.

### Resume Command

Resume only on explicit request with:

```bash
ssh airnet-server 'sudo systemctl enable --now invest-control-bot'
```

Do not start the legacy `investcontrol-server` backend at the same time.

## 2026-08-12 - Resume New Backend And Reconfirm Single Writer

### Request And Actions

- Explicitly stopped and disabled `invest-control-bot` on the legacy
  `investcontrol-server`; verified `inactive`, `disabled` and `MainPID=0`.
- Enabled and started `invest-control-bot` on `airnet-server` from the canonical
  `/home/ubuntu/apps/invest-control-bot` layout as `ubuntu:ubuntu`.

### Verification

- New service is `active/enabled`, `NRestarts=0`, and listens only on
  `127.0.0.1:8080`; PostgreSQL remains on `127.0.0.1:5432`.
- Startup applied zero pending migrations, passed Telegram and MAX API checks,
  retained the Telegram Worker webhook and ensured the MAX webhook at
  `https://investcontrol.org/max/webhook`.
- No WARN/ERROR/panic/fatal lines appeared after startup and the first scheduler
  interval.
- New-domain `/healthz` and `/` return 200; legacy proxied `/healthz` also
  returns 200.
- Protected app-role DB check returned `Europe/Moscow`, 18 users, 144 payments,
  115 subscriptions and 9 migrations.
- The old service remained `inactive/disabled` with PID 0, confirming exactly
  one production writer/callback handler.
- Go tests were skipped because no Go production code changed; this was an
  operational state transition plus documentation update.

## 2026-08-19 - Telegram Native Access-Link Hardening

### Goal And Diagnosis

- Fix connector links such as `https://web.telegram.org/a/#-100...`, which are
  routes inside Telegram Web and therefore open a browser instead of acting as
  reliable native Telegram deep links.
- Preserve the current production payment/subscription flow and the existing
  bot-created `t.me/+...` invite mechanism.
- Confirmed that raw Bot API IDs cannot produce a generic native channel URL:
  official `t.me/c/<channel>/<message_id>` links require a concrete message.

### Changes

- Added `internal/telegramlink`, a strict parser/normalizer for public links,
  invite links, private-message links, supported `tg://` forms and Telegram Web
  import hints.
- Telegram WebA/K/Z URLs now provide only a `chat_id`; they have no user-facing
  URL and are suppressed across payment notifications, subscription output and
  public recurring pages.
- Numeric private chat IDs no longer synthesize invalid bare `t.me/c/<id>`
  links. Domain capability now recognizes a bound private chat independently
  from the presence of a public URL.
- Connector creation canonicalizes official links and accepts a Telegram Web
  import only when the chat is already in `telegram_chats` and the bot has
  invite rights. The stored connector keeps the chat ID and drops the Web URL.
- Existing public `t.me` fallback and fresh bot-created invite behavior remain
  unchanged. Only recognized Telegram destinations are user-facing; foreign,
  malformed and Telegram Web legacy values are suppressed at read time.
- Static private invites are no longer exposed by public checkout, cancel,
  failed/pending or paid-result pages. Private access continues through the
  bot-created per-user invite after the provider callback.
- Connector create/update validates that the selected catalog chat has invite
  rights and matches any configured public/import destination, preventing an
  invite for chat B with a fallback URL for chat A.
- MTProto channel IDs are converted with Telegram's numeric offset formula,
  including IDs shorter than ten digits; reserved service routes such as
  `share`, `proxy` and `boost` are rejected as channel usernames.
- Replaced the old chat-ID backlog transcript with a current implementation,
  rollout and future `KeyboardButtonRequestChat` / `chat_shared` plan.

### Unit Coverage And Verification

- Added table tests for valid, legacy, malformed and lookalike Telegram links.
- Added regression tests for channel/domain resolution, admin Web import and
  chat matching, payment fallback/status pages, bot subscription fallback,
  recurring checkout HTML and cancel-page projection.
- Focused package tests passed.
- Race detector passed for all touched core packages:
  `go test -race ./internal/telegramlink ./internal/channelurl ./internal/telegramchat ./internal/domain ./internal/admin ./internal/app/payments ./internal/app/recurring ./internal/app ./internal/bot`.
- Full repository regression passed:
  `GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test ./...`.
- No migration, production data change, deployment, service restart or provider
  change was performed in this task.

### Remaining Rollout Work

- Deploy through the normal controlled path.
- Bind each private paid Telegram chat from the discovered-chat catalog and
  verify one new payment plus one "Моя подписка" recovery flow returns a fresh
  `t.me/+...` link.
- Backfill only validated chats; a legacy connector whose bot cannot see the
  target chat must be fixed by adding/re-authorizing the bot first.

## 2026-08-19 - Telegram Access-Link Production Deploy

### Deployment

- Re-ran the tracked-code gate before rollout: focused unit tests, affected
  package race tests, full `GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test ./...`,
  `go vet ./...` and `git diff --check` passed.
- Confirmed a quiet cutover point: no recent payment routes, established backend
  connections, active DB sessions or transactions older than 30 seconds.
- Confirmed the legacy host remained `inactive/disabled`, so production still
  had exactly one writer/callback handler.
- Saved the previous `2ed7bd2` binary and revision under
  `/home/ubuntu/apps/invest-control-bot/.deploy/rollback-2ed7bd2` with
  `ubuntu:ubuntu` ownership before replacement.
- Deployed clean tracked revision `5a0de97` through the simple-layout
  `scripts/deploy_vps.sh` path and restarted `invest-control-bot` once.

### Verification

- Local and remote binary SHA-256 matched:
  `04533602d1a427c5b32a58d8ac7bc3acc379a4e794a3930dd13d3e26c018678d`.
- Service is `active/running/enabled`, PID `381418`, `NRestarts=0`; app files
  and env remain owned by `ubuntu:ubuntu` and env mode remains `0600`.
- Startup applied zero migrations, passed Telegram and MAX API checks, and
  emitted no post-deploy WARN/ERROR/panic/fatal lines.
- Local/public `/healthz` returned `ok`; canonical root and legacy compatibility
  health returned HTTP 200. Telegram webhook reports `pending=0` and no last
  error.
- DB remained reachable with 9 migrations; no active foreign DB sessions were
  present at verification time.
- Six active connectors still store legacy Telegram Web URLs. Each public
  checkout returned HTTP 200 and none exposed `web.telegram.org`, proving the
  read-time safety boundary against real production rows.

### Follow-ups

- Bind/backfill each private connector through the verified Telegram chat
  catalog, then perform one controlled payment and one "Моя подписка" recovery
  check for a fresh per-user `t.me/+...` invite.
- The Go binary reports `vcs.modified=true` because untracked local artifacts,
  including SSH-key files, remain in the repository directory. They were not
  included in the binary, but release provenance should be cleaned before the
  next build and the key material must never be committed.
- One pre-existing MAX add-member failure for payment 162 used the configured
  channel-URL fallback successfully. This predates and is independent from the
  Telegram deployment, but merits a separate MAX access-flow check.
- Go tests were not repeated after this final documentation-only update; all
  production-code tests passed immediately before the deployed build.

## 2026-09-25 - Production Incident: Telegram Inbound Down Since 2026-09-22

### Goal

- Check production availability, scan logs for payment errors, and investigate
  a user report (user 42, Telegram, connector 99) about "payment problems".

### Findings

- `airnet-server`, the `invest-control-bot` service, Nginx, `/healthz` and
  PostgreSQL are healthy (`NRestarts=0`, no WARN/ERROR in the last hours).
- Robokassa callbacks already arrive directly at `https://investcontrol.org`
  (Nginx `investcontrol-org.access.log` shows `/payment/result` from Robokassa
  and browser `/payment/fail` with `auth.robokassa.ru` referrer). The payment
  callback path is not broken.
- The legacy host `192.144.13.87` (`xn--b1aghkfidhbthmd7l.xn--p1ai`) is fully
  down: no ICMP, TCP 443 closed, SSH timeout. Its DNS A record still points
  there.
- The Cloudflare Worker `telegram-bot-relay` still has
  `TELEGRAM_WEBHOOK_ORIGIN_URL` on the legacy hostname, so every Telegram
  update gets `502 Bad Gateway` from the Worker. `getWebhookInfo` reports
  `pending_update_count=5`, last error 2026-09-25 20:32 UTC. The last Telegram
  update that reached the backend was 2026-09-22 09:27:41 MSK; MAX webhook and
  outbound Telegram sends keep working.
- User 42 (subscription 160, `calendar_months`, autopay off, ends 2026-09-26
  12:47 MSK) received the expiry notice at 12:47 MSK with a `t.me/...?start=`
  renewal button. Any tap on it never reaches the bot because of the Worker
  502, so the user cannot start renewal. No payment rows were created.
- Airnet Nginx already serves the legacy hostname with a valid Let's Encrypt
  certificate (`sites-enabled/invest-control`, cert valid until 2026-10-26),
  so repointing the legacy DNS A record to `46.8.195.244` is an alternative
  fix that needs no Wrangler access.
- Local Wrangler OAuth token is expired and the session is non-interactive, so
  the Worker secret could not be changed from this session.
- Secondary finding (recurring): 12 rebills in the last 14 days got
  `OK<InvoiceID>` from Robokassa but never received a result callback. They
  stay `pending`, block any retry because of the unique pending-rebill index,
  and produce no `stale pending rebill` log/audit because
  `ReportStalePendingRebill` only covers `ShortDuration` connectors. Eight of
  those subscriptions (120-123, 125, 135, 150, 151) already expired and were
  revoked; four (161-164, user 43) expire on 2026-09-26 afternoon. This
  predates the legacy-host outage (first cases 2026-09-15) and looks like
  provider-side declines with no failure callback.
- Access-delivery errors in 14 days: three MAX `add chat member` failures
  (payments 290, 309, 310) that fell back to `max_channel_url`, and two revoke
  manual-check cases (subscriptions 117 Telegram `chat not found`, 120 MAX
  insufficient rights).

### Actions

- Read-only investigation only: no service restart, DB write, provider or
  Worker change. Go tests skipped because no production code changed.

### Follow-ups

- Restore Telegram inbound: run `wrangler login` interactively and set
  `TELEGRAM_WEBHOOK_ORIGIN_URL=https://investcontrol.org/telegram/webhook`,
  or repoint the legacy hostname A record to `46.8.195.244`.
- After restore, contact user 42 and re-send the renewal link if needed.
- Extend stale pending rebill reporting to long-period connectors and decide
  on retry/failure semantics for rebills without callback.

### Resolution (same day, 23:40 MSK)

- The owner ran `wrangler login` and set the Worker secret
  `TELEGRAM_WEBHOOK_ORIGIN_URL=https://investcontrol.org/telegram/webhook`.
- Telegram drained its queue within a minute: `pending_update_count` went
  5 -> 0, five `/telegram/webhook` POSTs returned 200, no WARN/ERROR.
- Four of the queued updates were user 42 tapping the renewal button; the bot
  processed them (`start_opened` for connector 99) so the user now sees the
  normal payment flow. The stale `502` text in `getWebhookInfo` is historical
  and clears on the next error/success cycle.
- The legacy hostname proxy is no longer on the Telegram path. Remaining
  legacy-host dependency is none for Telegram, MAX or Robokassa.

## 2026-09-25 - Stale Pending Rebill Visibility For Long-Period Connectors

### Change

- `ReportStalePendingRebill` in `internal/app/recurring/service.go` no longer
  returns early for non short-period connectors. Ordinary connectors report a
  pending rebill as stale once it is older than one hour
  (`longPeriodPendingRebillStaleAfter`) while the subscription is still
  active. Short-period behavior is unchanged. The warning log now carries
  `remaining` and `short_duration` instead of `ended_ago`.
- Observability only: the pending row stays pending, the scheduler still does
  not retry, and no user notification is sent. A `TODO:` next to the constant
  records the open retry/failure decision.
- Added `TestProcessRecurringRebills_RecordsStalePendingLongPeriodByAge`
  covering fresh vs stale pending, once-only reporting and no side effects.
- Docs updated: `docs/payments/robokassa-recurring.md`, `AGENTS.md`,
  `docs/context/progress.md`.

### Verification

- `GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test ./...` passed.
- `go vet ./internal/app/...` and `git diff --check` passed.
- Committed as `79fedd7` and deployed to `airnet-server` the same night via
  simple-layout `scripts/deploy_vps.sh` (one restart, 23:51 MSK). Previous
  binary `5a0de97` kept at `.deploy/rollback-pre-stale-202609252350`.
- Post-deploy: local/remote SHA-256 match
  (`bd2c725ad22ca50124bbd15e59eee82488a28a7ee23943f3992f8ad6768ea8d7`),
  service active with `NRestarts=0`, zero migrations applied, Telegram and MAX
  startup checks passed, `/healthz` 200 locally and publicly, Telegram webhook
  `pending=0`.
- The first scheduler sweep emitted exactly four `stale pending rebill without
  callback` warnings and four `rebill_pending_stale` audit events for user 43
  (subscriptions 161-164), and later sweeps did not repeat them, confirming
  the once-per-payment dedup on real production rows.

## 2026-09-26 - Telegram Webhook Direct To Origin, Worker Retired

### Hypothesis And Test

- The Cloudflare Worker existed because the previous RU-hosted VPS could not
  exchange traffic with Telegram. `airnet-server` is in Tashkent (UZ), so the
  relay should be unnecessary.
- Outbound was already direct: `TELEGRAM_API_BASE_URL` was commented out and
  `api.telegram.org` answers from the host in ~0.3s.
- Inbound test: `setWebhook` to `https://investcontrol.org/telegram/webhook`
  with the unchanged secret and allowed updates. Telegram resolved the host to
  `46.8.195.244`, and the first real updates arrived from Telegram's own IP
  `91.108.5.136` (previously Cloudflare `172.70.x`) with HTTP 200.

### Actions

- Enabled `TELEGRAM_WEBHOOK_PUBLIC_URL="https://investcontrol.org/telegram/webhook"`
  in the production env (backup `invest-control-bot.env.bak-20260926-webhook`,
  mode 0600 preserved) and restarted once at 00:05 MSK in a quiet window.
  Startup logged `telegram webhook is up to date`, zero migrations, MAX
  webhook ensured, `/healthz` 200, `NRestarts=0`.
- Worker deactivation is recorded below.
- Deactivated the Cloudflare Worker `telegram-bot-relay`: `workers_dev = false`
  in `wrangler.toml`, `wrangler deploy` reported "No targets deployed"
  (version `108a80fd`), and the Worker URL now returns HTTP 404 (Cloudflare
  error 1042). Secrets and code remain for a possible future reactivation.
- Go tests skipped: no Go code changed in this step (env, Worker config and
  docs only).

### Follow-ups

- Retire the legacy hostname server block and certificate on Airnet Nginx and
  the dead DNS A record.
- Remove `telegram-bot-relay/.wrangler/cache/wrangler-account.json` from git
  and ignore `.wrangler/`.
