package admin

import "github.com/Jopoleon/invest-control-bot/internal/domain"

func connectorActiveBadge(lang string, active bool) (string, string) {
	if active {
		return t(lang, "badge.connector.active"), "is-success"
	}
	return t(lang, "badge.connector.inactive"), "is-muted"
}

func paymentStatusBadge(lang string, status domain.PaymentStatus) (string, string) {
	switch status {
	case domain.PaymentStatusPaid:
		return t(lang, "badge.payment.paid"), "is-success"
	case domain.PaymentStatusFailed:
		return t(lang, "badge.payment.failed"), "is-danger"
	case domain.PaymentStatusPending:
		return t(lang, "badge.payment.pending"), "is-warning"
	default:
		return string(status), "is-muted"
	}
}

func subscriptionStatusBadge(lang string, status domain.SubscriptionStatus) (string, string) {
	switch status {
	case domain.SubscriptionStatusActive:
		return t(lang, "badge.subscription.active"), "is-success"
	case domain.SubscriptionStatusExpired:
		return t(lang, "badge.subscription.expired"), "is-warning"
	case domain.SubscriptionStatusRevoked:
		return t(lang, "badge.subscription.revoked"), "is-danger"
	default:
		return string(status), "is-muted"
	}
}

func autoPayBadge(lang string, enabled, configured bool) (string, string) {
	if !configured {
		return t(lang, "badge.autopay.unset"), "is-muted"
	}
	if enabled {
		return t(lang, "badge.autopay.enabled"), "is-accent"
	}
	return t(lang, "badge.autopay.disabled"), "is-warning"
}
