// Package periodpolicy centralizes period-derived timing rules for lifecycle
// and recurring orchestration.
//
// The business period model lives in internal/domain, while this package owns
// app-layer scheduling policy such as rebill lead times, notification
// suppression for short-lived subscriptions, and expiration grace windows.
package periodpolicy
