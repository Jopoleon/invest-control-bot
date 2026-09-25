# Telegram Bot Relay

> Status 2026-09-26: DEACTIVATED. Production runs in Tashkent and talks to
> Telegram directly in both directions (`TELEGRAM_WEBHOOK_PUBLIC_URL` points at
> `https://investcontrol.org/telegram/webhook`, `TELEGRAM_API_BASE_URL` is
> unset). The Worker is deployed with `workers_dev = false`, so its URL returns
> 404. To reactivate for a host where Telegram is blocked: set
> `workers_dev = true`, run `wrangler deploy`, verify the three secrets, then
> switch the app env back to the Worker URLs.

Cloudflare Worker relay for Telegram traffic.

It solves two separate network problems that can happen on RU-hosted VPS:

- outbound: the app cannot reach `https://api.telegram.org`
- inbound: Telegram cannot reach the app webhook URL on the RU domain

The Worker supports both directions:

- `/<bot api path>`: proxies Bot API requests from the app to `https://api.telegram.org`
- `/telegram/webhook`: receives Telegram webhook updates and forwards them to the app origin webhook

## Current Worker URL

```text
https://telegram-bot-relay.egortictac3.workers.dev
```

## Files

- `index.js`: Worker implementation
- `wrangler.toml`: Cloudflare Worker config

## Cloudflare Secrets

Set these secrets in Cloudflare Worker:

```bash
wrangler secret put TELEGRAM_BOT_TOKEN
wrangler secret put TELEGRAM_WEBHOOK_SECRET
wrangler secret put TELEGRAM_WEBHOOK_ORIGIN_URL
```

Values:

- `TELEGRAM_BOT_TOKEN`: same token as app env `TELEGRAM_BOT_TOKEN`
- `TELEGRAM_WEBHOOK_SECRET`: same secret as app env `TELEGRAM_WEBHOOK_SECRET`
- `TELEGRAM_WEBHOOK_ORIGIN_URL`: direct app webhook URL, currently
  `https://investcontrol.org/telegram/webhook`

Do not set `TELEGRAM_WEBHOOK_ORIGIN_URL` to the Worker URL. It must point to the real app origin, otherwise the Worker will proxy back to itself.

## App Env

For outbound Bot API relay:

```env
TELEGRAM_API_BASE_URL=https://telegram-bot-relay.egortictac3.workers.dev
```

For inbound webhook relay:

```env
TELEGRAM_WEBHOOK_PUBLIC_URL=https://telegram-bot-relay.egortictac3.workers.dev/telegram/webhook
```

Keep the app webhook secret unchanged:

```env
TELEGRAM_WEBHOOK_SECRET=<same value as Worker TELEGRAM_WEBHOOK_SECRET>
```

After changing `TELEGRAM_WEBHOOK_PUBLIC_URL`, restart the app. On startup it calls `setWebhook` and updates Telegram to the Worker URL.

## Deploy

From this directory:

```bash
wrangler deploy
```

If running in a non-interactive shell, set `CLOUDFLARE_API_TOKEN` first:

```bash
export CLOUDFLARE_API_TOKEN=...
wrangler deploy
```

If Wrangler tries to write logs into a read-only home directory, point its config/cache paths to writable locations:

```bash
export XDG_CONFIG_HOME=/tmp/wrangler-config
export XDG_CACHE_HOME=/tmp/wrangler-cache
```

## How It Works

### Outbound Bot API

The app sends requests to:

```text
https://telegram-bot-relay.egortictac3.workers.dev/bot<TOKEN>/getMe
```

The Worker:

- checks that `<TOKEN>` equals `TELEGRAM_BOT_TOKEN`
- forwards the request to `https://api.telegram.org/bot<TOKEN>/getMe`
- returns Telegram response to the app
- aborts slow upstream requests after 8 seconds and returns JSON `502` with
  `error_code: 502` and a readable `description`

For Bot API routes the Worker intentionally returns Telegram-shaped error JSON.
The Go Telegram SDK expects `ok`, `error_code`, and `description`; returning a
custom-only payload makes application logs collapse to unhelpful errors such as
`error response from telegram for method sendMessage, 0`.

Requests with any other bot token return `403`.

### Inbound Webhook

Telegram sends updates to:

```text
https://telegram-bot-relay.egortictac3.workers.dev/telegram/webhook
```

The Worker:

- accepts only `POST`
- checks `X-Telegram-Bot-Api-Secret-Token`
- forwards the original update body to `TELEGRAM_WEBHOOK_ORIGIN_URL`
- forwards the same secret header to the app
- returns the app response back to Telegram
- aborts slow origin/upstream requests after 8 seconds and returns JSON `502`
  instead of letting the request hang until the caller times out

This keeps the app webhook handler unchanged.

## Production Switch Procedure

1. Deploy Worker:

```bash
cd telegram-bot-relay
wrangler deploy
```

2. Ensure Worker secrets are set:

```bash
wrangler secret put TELEGRAM_BOT_TOKEN
wrangler secret put TELEGRAM_WEBHOOK_SECRET
wrangler secret put TELEGRAM_WEBHOOK_ORIGIN_URL
```

3. Update app env on the VPS:

```env
TELEGRAM_API_BASE_URL=https://telegram-bot-relay.egortictac3.workers.dev
TELEGRAM_WEBHOOK_PUBLIC_URL=https://telegram-bot-relay.egortictac3.workers.dev/telegram/webhook
```

4. Restart service:

```bash
sudo systemctl restart invest-control-bot
```

5. Check app logs:

```bash
journalctl -u invest-control-bot -n 100 --no-pager
```

Expected:

- `telegram api ping ok`
- `telegram webhook updated` or `telegram webhook is up to date`
- `service started`

6. Check webhook info through relay:

```bash
curl -sS "$TELEGRAM_API_BASE_URL/bot$TELEGRAM_BOT_TOKEN/getWebhookInfo"
```

Expected:

- `url` is `https://telegram-bot-relay.egortictac3.workers.dev/telegram/webhook`
- `pending_update_count` decreases after updates are delivered
- no fresh `last_error_message`

## Smoke Checks

Check outbound relay:

```bash
curl -sS "$TELEGRAM_API_BASE_URL/bot$TELEGRAM_BOT_TOKEN/getMe"
```

Check that the Worker is not an open proxy:

```bash
curl -i "https://telegram-bot-relay.egortictac3.workers.dev/bot000:bad/getMe"
```

Expected: `403`.

Check webhook route rejects non-POST:

```bash
curl -i "https://telegram-bot-relay.egortictac3.workers.dev/telegram/webhook"
```

Expected: `405`.

If Telegram or the origin is unreachable from the Worker, the relay should fail
fast. Bot API proxy routes return Telegram-compatible errors, for example:

```json
{
  "ok": false,
  "error_code": 502,
  "description": "Telegram relay upstream timeout after 8000ms",
  "error": "upstream_timeout",
  "timeout_ms": 8000
}
```

Webhook/origin routes return generic Worker errors, for example:

```json
{"ok":false,"error":"upstream_timeout","timeout_ms":8000}
```

or:

```json
{"ok":false,"error":"upstream_fetch_failed","message":"..."}
```

This is intentional. The app can then enter degraded Telegram startup mode
quickly instead of waiting for a long network timeout.

## Current Incident Pattern

The observed failure on `2026-04-28`:

- outbound `getMe` via Worker returned `200`
- direct `api.telegram.org:443` from VPS timed out
- Telegram `getWebhookInfo` showed `pending_update_count > 0`
- Telegram `last_error_message` was `Connection timed out`
- nginx/app logs did not show Telegram POST requests

That means outbound relay was working, but inbound Telegram-to-origin webhook delivery was failing. The fix is to set `TELEGRAM_WEBHOOK_PUBLIC_URL` to the Worker `/telegram/webhook` route and let Worker forward updates to the origin.
