package admin

import (
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func TestBuildRecurringSummary_EnabledWithoutConsent(t *testing.T) {
	got := buildRecurringSummary("ru", true, true, nil, map[int64]string{}, nil, nil)
	if got.HealthClass != "is-danger" {
		t.Fatalf("HealthClass = %q, want %q", got.HealthClass, "is-danger")
	}
}

func TestBuildRecurringSummary_UsesLatestConsent(t *testing.T) {
	now := time.Now().UTC()
	got := buildRecurringSummary("ru", true, true, []domain.RecurringConsent{
		{ConnectorID: 2, AcceptedAt: now},
	}, map[int64]string{2: "connector-2"}, nil, nil)
	if got.LastConsentConnector != "connector-2" {
		t.Fatalf("LastConsentConnector = %q, want %q", got.LastConsentConnector, "connector-2")
	}
	if got.HealthClass != "is-success" {
		t.Fatalf("HealthClass = %q, want %q", got.HealthClass, "is-success")
	}
}

func TestBuildRecurringSummary_UsesRebillState(t *testing.T) {
	now := time.Now().UTC()
	got := buildRecurringSummary("ru", true, true, nil, map[int64]string{}, []domain.Payment{
		{
			SubscriptionID:  11,
			ParentPaymentID: 1,
			Status:          domain.PaymentStatusFailed,
			CreatedAt:       now,
		},
	}, []domain.Subscription{
		{ID: 11, AutoPayEnabled: true},
	})
	if got.LastRebillLabel != "последний rebill с ошибкой" {
		t.Fatalf("LastRebillLabel = %q", got.LastRebillLabel)
	}
	if got.FailedAttempts != 1 {
		t.Fatalf("FailedAttempts = %d", got.FailedAttempts)
	}
}
