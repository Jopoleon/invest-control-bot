// Package messenger defines transport-neutral messenger contracts.
//
// It provides common inbound event structs, outbound message shapes, and the
// sender interface used by the bot and app layers to work with Telegram and MAX
// without depending on transport-specific request and response DTOs.
//
// This package is intentionally thin: it is a boundary contract, not a place
// for business logic or storage models.
package messenger
