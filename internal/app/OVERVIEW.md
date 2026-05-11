# internal/app Overview

`internal/app` is the application/runtime layer. It wires configuration,
storage, payment providers, messenger transports, HTTP routes, public pages, and
background jobs.

## Responsibilities

- HTTP server and route registration.
- Payment callbacks and payment status pages.
- Public recurring checkout/cancel pages.
- Recurring scheduler and manual rebill orchestration.
- Subscription lifecycle jobs: reminders, expiry, revoke/access cleanup.
- Store opening and startup migrations.
- App-level notification and audit helpers.

## Important Subpackages

- `payments` - payment activation and provider-centered business flow.
- `recurring` - recurring page state and rebill orchestration.
- `subscriptions` - subscription lifecycle behavior.
- `periodpolicy` - recurring/lifecycle timing, including short-duration policy.

## Invariants

- Payment success comes from provider callback handling only.
- Recurring rebill success comes from provider result callback only.
- Pending rebills are not successful renewals.
- Subscription extension uses `max(now, current period end)` where applicable.
- Short-period recurring behavior must stay centralized in `periodpolicy`.

## Checks

Run the full suite after app-layer changes:

```bash
GOCACHE=/tmp/go-build go test ./...
```

For focused changes, also run relevant tests under `internal/app/periodpolicy`,
`internal/app/payments`, `internal/app/recurring`, and
`internal/app/subscriptions`.

## Related Docs

- `docs/context/project-context.md`
- `docs/planning/implementation-plan.md`
- `docs/architecture/app-refactor.md`
- `docs/payments/flow-ru.md`
- `docs/payments/robokassa-recurring.md`
