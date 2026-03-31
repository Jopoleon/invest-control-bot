// Package recurring contains app-layer recurring payment orchestration.
//
// The package owns scheduled rebill eligibility, provider-side rebill
// triggering, and the business state behind public recurring cancel pages.
// HTTP routing, token parsing, and template rendering still remain in the root
// app package for now because they are more tightly coupled to transport and
// page-specific HTTP concerns.
package recurring
