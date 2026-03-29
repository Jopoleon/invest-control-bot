// Package app wires the HTTP application runtime.
//
// It owns server startup and shutdown, route registration, webhook handlers,
// public compliance pages, payment callback processing, recurring lifecycle
// jobs, and app-level notification/audit side effects.
//
// Business logic in this package should stay transport-neutral where possible.
// Messenger-specific delivery must go through the linked-identity helpers
// instead of assuming Telegram-only identifiers on business records.
package app
