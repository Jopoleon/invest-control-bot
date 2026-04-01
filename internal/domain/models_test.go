package domain

import (
	"testing"
	"time"
)

func TestConnectorSubscriptionEndsAt_DurationMode(t *testing.T) {
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	connector := Connector{PeriodMode: ConnectorPeriodModeDuration, PeriodSeconds: 120}

	got := connector.SubscriptionEndsAt(start)
	want := start.Add(120 * time.Second)
	if !got.Equal(want) {
		t.Fatalf("SubscriptionEndsAt(duration)=%s want=%s", got, want)
	}
}

func TestConnectorSubscriptionEndsAt_CalendarMonthsMode(t *testing.T) {
	start := time.Date(2026, 1, 31, 10, 0, 0, 0, time.UTC)
	connector := Connector{PeriodMode: ConnectorPeriodModeCalendarMonths, PeriodMonths: 1}

	got := connector.SubscriptionEndsAt(start)
	want := start.AddDate(0, 1, 0)
	if !got.Equal(want) {
		t.Fatalf("SubscriptionEndsAt(calendar_months)=%s want=%s", got, want)
	}
}

func TestConnectorSubscriptionEndsAt_FixedDeadlineMode(t *testing.T) {
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	fixedEndsAt := time.Date(2026, 4, 30, 21, 0, 0, 0, time.UTC)
	connector := Connector{PeriodMode: ConnectorPeriodModeFixedDeadline, FixedEndsAt: &fixedEndsAt}

	got := connector.SubscriptionEndsAt(start)
	if !got.Equal(fixedEndsAt) {
		t.Fatalf("SubscriptionEndsAt(fixed_deadline)=%s want=%s", got, fixedEndsAt)
	}
	if connector.SupportsRecurring() {
		t.Fatal("SupportsRecurring(fixed_deadline)=true want false")
	}
}

func TestConnectorSubscriptionEndsAt_LegacyFallback(t *testing.T) {
	start := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	connector := Connector{PeriodMode: ConnectorPeriodModeDuration, PeriodSeconds: 90}

	got := connector.SubscriptionEndsAt(start)
	want := start.Add(90 * time.Second)
	if !got.Equal(want) {
		t.Fatalf("SubscriptionEndsAt(duration)=%s want=%s", got, want)
	}

	connector = Connector{}
	got = connector.SubscriptionEndsAt(start)
	want = start.Add(30 * 24 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("SubscriptionEndsAt(default duration)=%s want=%s", got, want)
	}
}
