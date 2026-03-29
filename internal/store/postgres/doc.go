// Package postgres provides the PostgreSQL store implementation.
//
// It is the primary persistence layer of the service and should stay aligned
// with the canonical SQL schema in migrations. Query and write paths here must
// preserve business invariants around payments, subscriptions, recurring
// consent, and messenger identity resolution.
package postgres
