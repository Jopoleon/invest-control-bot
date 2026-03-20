package admin

import (
	"testing"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
)

func TestBuildRecurringSummary_EnabledWithoutConsent(t *testing.T) {
	got := buildRecurringSummary("ru", true, true, nil, map[int64]string{})
	if got.HealthClass != "is-danger" {
		t.Fatalf("HealthClass = %q, want %q", got.HealthClass, "is-danger")
	}
}

func TestBuildRecurringSummary_UsesLatestConsent(t *testing.T) {
	now := time.Now().UTC()
	got := buildRecurringSummary("ru", true, true, []domain.RecurringConsent{
		{ConnectorID: 2, AcceptedAt: now},
	}, map[int64]string{2: "connector-2"})
	if got.LastConsentConnector != "connector-2" {
		t.Fatalf("LastConsentConnector = %q, want %q", got.LastConsentConnector, "connector-2")
	}
	if got.HealthClass != "is-success" {
		t.Fatalf("HealthClass = %q, want %q", got.HealthClass, "is-success")
	}
}
