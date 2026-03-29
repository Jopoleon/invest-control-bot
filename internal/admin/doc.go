// Package admin implements the server-rendered administration panel.
//
// It contains session-based authentication, localized HTML pages, CSV exports,
// manual support actions, and reporting views over users, payments,
// subscriptions, audit events, and recurring state.
//
// Admin pages should prefer internal user IDs as the canonical identity and
// resolve messenger-specific projections only when they are needed for display
// or transport actions.
package admin
