// Package store declares the persistence contracts used by the application.
//
// It defines the storage interfaces consumed by app, bot, and admin logic so
// business code can work against one repository boundary while PostgreSQL and
// in-memory implementations evolve independently.
package store
