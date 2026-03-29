// Package bot contains the user-facing messenger flow.
//
// It implements onboarding, consent capture, checkout preparation, menu and
// autopay actions, and audit logging on top of transport-neutral inbound and
// outbound messenger contracts.
//
// This package should not depend on Telegram DTOs directly. Concrete messenger
// adapters must map transport updates into internal messenger events before they
// reach the bot handler.
package bot
