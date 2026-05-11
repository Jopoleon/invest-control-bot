# internal/bot Overview

`internal/bot` owns user-facing messenger flows after raw transport updates have
been mapped into internal events.

## Responsibilities

- `/start <payload>` onboarding.
- Registration and consent flow.
- Payment preparation and user-facing payment actions.
- Subscription, payment history, and autopay menus.
- Routing internal messenger messages/actions into use-case behavior.
- Telegram update mapping where legacy Telegram DTOs still enter this package.

## Boundaries

- Business flow should use `messenger.UserIdentity` and internal `user_id`
  resolution, not raw Telegram-only assumptions.
- Transport-specific DTOs should stay at adapter/router edges.
- MAX support should reuse the same user-facing business flow rather than fork
  Telegram logic.

## Invariants

- Keep the existing Telegram product flow stable.
- Do not reintroduce `TelegramID` into `domain.Payment` or
  `domain.Subscription`.
- Missing legal documents, connector access mismatch, and recurring choices
  should remain explicit user-facing states.
- Payment success must still be confirmed by app/payment callback flow, not by
  bot navigation.

## Checks

Prioritize focused tests in `internal/bot` for:

- callback/action routing;
- payment and recurring choices;
- subscription overview;
- payment history;
- existing subscription behavior.

Full check:

```bash
GOCACHE=/tmp/go-build go test ./...
```

## Related Docs

- `docs/context/project-context.md`
- `docs/architecture/max-decomposition.md`
- `docs/architecture/refactoring-and-tests.md`
- `docs/payments/flow-ru.md`
