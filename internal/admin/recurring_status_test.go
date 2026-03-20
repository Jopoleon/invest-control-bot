package admin

import (
	"testing"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
)

func TestBuildRecurringPaymentState_CountsFailuresAndTracksLatest(t *testing.T) {
	now := time.Now().UTC()
	state := buildRecurringPaymentState([]domain.Payment{
		{
			ID:              1,
			SubscriptionID:  77,
			ParentPaymentID: 10,
			Status:          domain.PaymentStatusFailed,
			CreatedAt:       now.Add(-2 * time.Hour),
		},
		{
			ID:              2,
			SubscriptionID:  77,
			ParentPaymentID: 10,
			Status:          domain.PaymentStatusPending,
			CreatedAt:       now.Add(-1 * time.Hour),
		},
		{
			ID:              3,
			SubscriptionID:  88,
			ParentPaymentID: 10,
			Status:          domain.PaymentStatusFailed,
			CreatedAt:       now,
		},
	}, 77)

	if state.FailedAttempts != 1 {
		t.Fatalf("FailedAttempts = %d, want 1", state.FailedAttempts)
	}
	if state.LastStatus != domain.PaymentStatusPending {
		t.Fatalf("LastStatus = %q, want %q", state.LastStatus, domain.PaymentStatusPending)
	}
}

func TestRecurringRetryBadge_ExhaustedAfterThreeFailures(t *testing.T) {
	label, className := recurringRetryBadge("ru", true, recurringPaymentState{FailedAttempts: 3, HasAttempts: true, LastStatus: domain.PaymentStatusFailed})
	if label != "ошибок: 3/3" {
		t.Fatalf("label = %q", label)
	}
	if className != "is-danger" {
		t.Fatalf("class = %q", className)
	}
}

func TestMatchesRecurringRetryFilter(t *testing.T) {
	state := recurringPaymentState{FailedAttempts: 2, HasAttempts: true, LastStatus: domain.PaymentStatusFailed}
	if !matchesRecurringRetryFilter(recurringRetryFilterFailed, true, state) {
		t.Fatalf("failed filter should match")
	}
	if matchesRecurringRetryFilter(recurringRetryFilterExhausted, true, state) {
		t.Fatalf("exhausted filter should not match")
	}
	if matchesRecurringRetryFilter(recurringRetryFilterFailed, false, state) {
		t.Fatalf("disabled autopay should not match retry filter")
	}
}
