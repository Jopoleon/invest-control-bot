# Robokassa Error 29 After Receipt Fiscalization - 2026-05-26

## Summary

After adding Robokassa fiscalization via the `Receipt` parameter, operators
continued to see Robokassa error `29` on checkout:

> Оплата счетов недоступна. Проверьте активацию магазина и использование
> технических настроек.

Robokassa support pointed to the usual causes of error `29`:

- wrong `MerchantLogin`;
- wrong password #1;
- wrong password #2, usually together with password #1 mismatch;
- test mode enabled while production passwords are used;
- if settings are correct, regenerate passwords #1 and #2 in the Robokassa
  technical settings and save/confirm the change.

The shop credentials were regenerated twice, but the issue persisted. The
payment flow started working after fixing how `Receipt` is encoded in our
request.

## Root Cause

The integration was signing `Receipt` correctly using the URL-encoded JSON value,
but it sent the `Receipt` request parameter as raw JSON and relied on the
transport layer (`url.Values`) to encode it once.

Robokassa documentation says:

- `Receipt` participates in the signature as
  `MerchantLogin:OutSum:InvId:Receipt:Password#1`;
- before adding it to the signature, the `Receipt` value must be URL-encoded;
- in payment request examples, the `Receipt` field itself is already
  URL-encoded before being placed into the form/request.

That means our request must store the once-encoded receipt as the value of the
`Receipt` parameter. The query/form transport then encodes that value again.
The final wire representation is therefore double-encoded, while the signature
uses the once-encoded value.

## Fix

Changed Robokassa checkout and recurring rebill generation so that:

- raw receipt JSON is still built as before;
- `encodedReceipt := url.QueryEscape(rawReceipt)` is used in the signature;
- the `Receipt` request parameter is set to `encodedReceipt`, not raw JSON;
- `url.Values.Encode()` then applies transport encoding, producing the
  double-encoded wire value expected by Robokassa examples.

Affected code:

- `internal/payment/robokassa.go`
- `internal/payment/robokassa_test.go`

## Verification

Automated:

```bash
GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test ./internal/payment ./internal/config ./internal/app ./...
```

Manual / production:

- Deployed the fix.
- Created a fresh payment link.
- Robokassa checkout opened correctly.
- Payment flow was confirmed by the operator as working.

## Notes

Password #3 in Robokassa technical settings was checked during the incident.
It is not used by the current integration path:

- password #1 signs checkout requests and success redirects;
- password #2 verifies ResultURL/provider callbacks and OpState requests;
- password #3 is for other Robokassa API scenarios such as refunds and is not
  required for current checkout/recurring flow.

Failed recurring attempts around payment IDs `93-95` were a separate issue:
Robokassa `/Merchant/Recurring` returned HTTP 500 with
`Recurring: Internal ERROR occurred`. That is provider-side recurring failure
visibility, not checkout error `29`.

