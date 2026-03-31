// Package subscriptions contains app-layer subscription lifecycle services.
//
// The package owns reminder delivery, expiry notices, expiration processing,
// and renewal-notification composition. It is intentionally separated from the
// root app package so these business flows can evolve and be tested without
// pulling in HTTP routing or scheduler setup.
package subscriptions
