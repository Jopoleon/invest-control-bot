package periodpolicy

import (
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func TestResolveShortDurationTiming_ForFourMinutePeriod(t *testing.T) {
	timing := Resolve(domain.Connector{
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 4 * 60,
	})

	if !timing.ShortDuration {
		t.Fatal("ShortDuration=false want true")
	}
	if timing.FirstRebillLead != 80*time.Second {
		t.Fatalf("FirstRebillLead=%s want=%s", timing.FirstRebillLead, 80*time.Second)
	}
	if timing.SecondRebillLead != 24*time.Second {
		t.Fatalf("SecondRebillLead=%s want=%s", timing.SecondRebillLead, 24*time.Second)
	}
	if timing.ExpiryGrace != 40*time.Second {
		t.Fatalf("ExpiryGrace=%s want=%s", timing.ExpiryGrace, 40*time.Second)
	}
}

func TestResolveNormalDurationTiming_IsEmpty(t *testing.T) {
	timing := Resolve(domain.Connector{
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: int64((24 * time.Hour) / time.Second),
	})

	if timing.ShortDuration {
		t.Fatal("ShortDuration=true want false")
	}
	if timing.RebillAttemptOrdinal(time.Now().UTC(), time.Now().UTC().Add(time.Hour)) != 0 {
		t.Fatal("RebillAttemptOrdinal(normal duration) should stay 0")
	}
}
