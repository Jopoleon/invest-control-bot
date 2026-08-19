# Telegram Chat ID And Access Links

## Problem

`https://web.telegram.org/a/#-100...` is a route inside Telegram Web, not a
cross-client deep link. Sending it as the paid-access button opens the browser
client instead of reliably opening the native Telegram application.

A Bot API chat ID such as `-100...` also cannot be converted into a generic
`t.me/c/<id>` channel link. Official `t.me/c/<channel>/<message_id>` links point
to a concrete private message and require a message ID.

## Implemented Safety Boundary

The code now distinguishes Telegram chat identity from user-facing navigation:

- public chats use canonical `https://t.me/<username>` links;
- private access uses a fresh bot-created `https://t.me/+<invite_hash>` link;
- legacy `telegram.me`, `joinchat`, and supported `tg://resolve`/`tg://join`
  inputs are canonicalized to `t.me`;
- `web.telegram.org/{a,k,z}/#-100...` is accepted only as an import hint for
  the Bot API chat ID and is never rendered in a button or public page;
- numeric chat IDs no longer synthesize an invalid bare `t.me/c/<id>` URL;
- creating a connector from a Telegram Web URL succeeds only when that chat is
  already present in the local `telegram_chats` catalog and the bot is an
  administrator with `can_invite_users`;
- if creation of a private invite fails, the payment and subscription flows do
  not fall back to Telegram Web. They retain the existing audit/error path.
- public checkout, cancel and payment-result pages expose public usernames only;
  they never disclose a static private invite as a substitute for bot delivery;
- Bot API channel references are derived from MTProto IDs with Telegram's
  numeric offset formula, and reserved `t.me` service routes are not accepted
  as public channel usernames;
- selecting a discovered chat is rejected when it conflicts with the
  connector's configured public/import destination.

This needs no database migration. Existing rows remain readable: a stored Web
URL can contribute its `chat_id` at runtime, but it is suppressed as a public
URL. Existing rows should still be normalized operationally after deployment.

Rollout status: revision `5a0de97` was deployed to `airnet-server` on
2026-08-19. All six active legacy-Web connector checkout pages returned 200
without exposing `web.telegram.org`; catalog binding/backfill remains pending.

## Existing Operational Flow

Telegram does not provide a method to list every chat where a bot is an admin,
nor can a private `t.me/+...` invite hash be reversed into a chat ID.

The current supported flow is:

1. Add the bot to the paid channel/group as administrator.
2. Grant `can_invite_users`; grant `can_restrict_members` as well so expiry
   revocation can work.
3. Trigger a `my_chat_member` update by adding/re-adding the bot or changing its
   rights.
4. Select the discovered chat in the connector's Telegram chat dropdown.
5. Verify a payment or "Моя подписка" action returns a fresh `t.me/+...` invite.

`createChatInviteLink` currently creates a per-access link with a 36-hour TTL
and `member_limit=1`, stores it in `telegram_invite_links`, and allows lifecycle
code to revoke it later.

## Remaining Product Work

The preferred long-term operator UX is an explicit "Подключить Telegram-чат"
flow using `KeyboardButtonRequestChat` / `chat_shared`:

1. Create a signed, short-lived connector binding request.
2. Ask the operator to select a channel or group in Telegram's native UI.
3. Receive `chat_shared.chat_id`.
4. Validate `getChat` and the bot's administrator rights server-side.
5. Bind the verified chat to the connector and prevent replay of the request.

Keep `my_chat_member` discovery and the current dropdown as a compatible manual
path. A static invite URL is not a substitute for a verified chat binding,
because it cannot support reliable invite creation, revocation, or membership
lifecycle operations.

## Verification

Unit coverage exists for parsing/canonicalization, ID conversion, reserved and
unsafe-host rejection, Web/static-invite suppression, admin import/chat-match
validation, connector capability, payment fallback/status pages, bot
subscription fallback, public recurring checkout and cancel-page projection.

Main regression command:

```bash
GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test ./...
```

References:

- <https://core.telegram.org/api/links>
- <https://core.telegram.org/bots/api#createchatinvitelink>
- <https://core.telegram.org/bots/api#keyboardbuttonrequestchat>
- <https://core.telegram.org/bots/features#chat-and-user-selection>
