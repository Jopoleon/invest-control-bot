// Package payments contains app-layer payment business services.
//
// The package currently owns payment activation and recurring-failure
// notification behavior. HTTP handlers and payment status page rendering still
// remain in the root app package for now because they are more tightly coupled
// to routing, templates, and HTTP-only DTOs.
package payments
