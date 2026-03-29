// Package memory provides an in-memory store implementation for local flows and
// tests.
//
// It mirrors the store contracts closely enough to exercise business behavior
// without PostgreSQL, but PostgreSQL remains the source of truth for schema and
// persistence semantics.
package memory
