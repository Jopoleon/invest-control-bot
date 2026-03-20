package admin

import (
	"fmt"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
)

type recurringPaymentState struct {
	FailedAttempts int
	HasAttempts    bool
	LastAttemptAt  time.Time
	LastStatus     domain.PaymentStatus
}

type recurringRetryFilter string

const (
	recurringRetryFilterNone      recurringRetryFilter = "none"
	recurringRetryFilterPending   recurringRetryFilter = "pending"
	recurringRetryFilterFailed    recurringRetryFilter = "failed"
	recurringRetryFilterExhausted recurringRetryFilter = "exhausted"
)

func buildRecurringPaymentState(payments []domain.Payment, subscriptionID int64) recurringPaymentState {
	state := recurringPaymentState{}
	if subscriptionID <= 0 {
		return state
	}

	for _, payment := range payments {
		if payment.SubscriptionID != subscriptionID || payment.ParentPaymentID <= 0 {
			continue
		}
		state.HasAttempts = true
		if payment.Status == domain.PaymentStatusFailed {
			state.FailedAttempts++
		}
		if state.LastAttemptAt.IsZero() || payment.CreatedAt.After(state.LastAttemptAt) || (payment.CreatedAt.Equal(state.LastAttemptAt) && payment.ID > 0) {
			state.LastAttemptAt = payment.CreatedAt
			state.LastStatus = payment.Status
		}
	}

	return state
}

func recurringRetryBadge(lang string, autoPayEnabled bool, state recurringPaymentState) (string, string) {
	if !autoPayEnabled {
		return t(lang, "churn.recurring.disabled"), "is-muted"
	}
	if state.LastStatus == domain.PaymentStatusPending {
		return t(lang, "churn.recurring.pending"), "is-accent"
	}
	if state.FailedAttempts >= 3 {
		return fmt.Sprintf(t(lang, "churn.recurring.failed_n"), state.FailedAttempts), "is-danger"
	}
	if state.FailedAttempts > 0 {
		return fmt.Sprintf(t(lang, "churn.recurring.failed_n"), state.FailedAttempts), "is-warning"
	}
	return t(lang, "churn.recurring.not_started"), "is-muted"
}

func matchesAutoPayFilter(filter string, enabled bool, configured bool) bool {
	switch filter {
	case "enabled":
		return configured && enabled
	case "disabled":
		return configured && !enabled
	case "unset":
		return !configured
	default:
		return true
	}
}

func matchesRecurringRetryFilter(filter recurringRetryFilter, autoPayEnabled bool, state recurringPaymentState) bool {
	switch filter {
	case recurringRetryFilterNone:
		return autoPayEnabled && !state.HasAttempts
	case recurringRetryFilterPending:
		return autoPayEnabled && state.LastStatus == domain.PaymentStatusPending
	case recurringRetryFilterFailed:
		return autoPayEnabled && state.FailedAttempts > 0 && state.FailedAttempts < 3
	case recurringRetryFilterExhausted:
		return autoPayEnabled && state.FailedAttempts >= 3
	default:
		return true
	}
}
