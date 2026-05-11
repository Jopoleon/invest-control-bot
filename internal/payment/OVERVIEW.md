# internal/payment Overview

`internal/payment` contains payment provider integrations and payment-provider
contracts.

## Responsibilities

- Mock provider for local development.
- Robokassa checkout and recurring/rebill integration.
- Provider request signing and response verification.
- Provider-level metadata logging where needed for operations.

## Invariants

- A provider redirect page is not payment confirmation.
- Robokassa `Merchant/Recurring` request/response is not rebill success by
  itself.
- Successful recurring renewal requires provider result callback handling.
- Recurring consent and user opt-in are app/domain responsibilities; provider
  code must not silently enable recurring.
- Provider test/production mode must be explicit in production configuration.

## Checks

Run provider and app callback tests after changes:

```bash
GOCACHE=/tmp/go-build go test ./internal/payment ./internal/app/...
```

Use the full suite before handoff:

```bash
GOCACHE=/tmp/go-build go test ./...
```

## Related Docs

- `docs/payments/flow-ru.md`
- `docs/payments/robokassa-recurring.md`
- `docs/context/project-context.md`
