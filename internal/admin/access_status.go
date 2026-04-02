package admin

import (
	"strconv"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func buildPaymentAccessStatus(lang string, payment domain.Payment, events []domain.AuditEvent) (string, string) {
	if payment.Status != domain.PaymentStatusPaid {
		return t(lang, "badge.ops.not_applicable"), "is-muted"
	}
	for _, event := range events {
		if auditDetailInt64(event.Details, "payment_id") != payment.ID {
			continue
		}
		switch event.Action {
		case domain.AuditActionAccessDeliveryFailed:
			return t(lang, "badge.ops.access_issue"), "is-danger"
		case domain.AuditActionPaymentAccessReady:
			return t(lang, "badge.ops.access_ready"), "is-success"
		}
	}
	return t(lang, "badge.ops.not_checked"), "is-muted"
}

func buildSubscriptionAccessStatus(lang string, sub domain.Subscription, events []domain.AuditEvent) (string, string) {
	if sub.Status == domain.SubscriptionStatusActive {
		if sub.PaymentID <= 0 {
			return t(lang, "badge.ops.not_checked"), "is-muted"
		}
		for _, event := range events {
			if auditDetailInt64(event.Details, "payment_id") != sub.PaymentID {
				continue
			}
			switch event.Action {
			case domain.AuditActionAccessDeliveryFailed:
				return t(lang, "badge.ops.access_issue"), "is-danger"
			case domain.AuditActionPaymentAccessReady:
				return t(lang, "badge.ops.access_ready"), "is-success"
			}
		}
		return t(lang, "badge.ops.not_checked"), "is-muted"
	}

	for _, event := range events {
		if auditDetailInt64(event.Details, "subscription_id") != sub.ID {
			continue
		}
		switch event.Action {
		case domain.AuditActionSubscriptionRevokeManualCheck:
			return t(lang, "badge.ops.manual_check"), "is-danger"
		case domain.AuditActionSubscriptionRevokedFromChat:
			return t(lang, "badge.ops.revoked"), "is-success"
		case domain.AuditActionSubscriptionRevokeFailed:
			return t(lang, "badge.ops.retry_scheduled"), "is-warning"
		case domain.AuditActionSubscriptionExpired:
			if auditDetailValue(event.Details, "replacement_active") == "true" {
				return t(lang, "badge.ops.replacement_active"), "is-muted"
			}
		}
	}

	if sub.Status == domain.SubscriptionStatusExpired || sub.Status == domain.SubscriptionStatusRevoked {
		return t(lang, "badge.ops.not_checked"), "is-muted"
	}
	return t(lang, "badge.ops.not_applicable"), "is-muted"
}

func auditDetailInt64(details, key string) int64 {
	value, _ := strconv.ParseInt(auditDetailValue(details, key), 10, 64)
	return value
}

func auditDetailValue(details, key string) string {
	prefix := strings.TrimSpace(key) + "="
	for _, part := range strings.Split(details, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(part, prefix))
		}
	}
	return ""
}
