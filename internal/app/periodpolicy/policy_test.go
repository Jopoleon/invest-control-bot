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
	if timing.ThirdRebillLead != 12*time.Second {
		t.Fatalf("ThirdRebillLead=%s want=%s", timing.ThirdRebillLead, 12*time.Second)
	}
	if timing.ExpiryGrace != 40*time.Second {
		t.Fatalf("ExpiryGrace=%s want=%s", timing.ExpiryGrace, 40*time.Second)
	}
}

func TestResolveShortDurationTiming_ForThreeHourPeriod(t *testing.T) {
	timing := Resolve(domain.Connector{
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: int64((3 * time.Hour) / time.Second),
	})

	if !timing.ShortDuration {
		t.Fatal("ShortDuration=false want true")
	}
	if !timing.CustomRebillTiming {
		t.Fatal("CustomRebillTiming=false want true")
	}
	if timing.FirstRebillLead != time.Hour {
		t.Fatalf("FirstRebillLead=%s want=%s", timing.FirstRebillLead, time.Hour)
	}
	if timing.SecondRebillLead != 18*time.Minute {
		t.Fatalf("SecondRebillLead=%s want=%s", timing.SecondRebillLead, 18*time.Minute)
	}
	if timing.ThirdRebillLead != 9*time.Minute {
		t.Fatalf("ThirdRebillLead=%s want=%s", timing.ThirdRebillLead, 9*time.Minute)
	}
	if timing.ExpiryGrace != 5*time.Minute {
		t.Fatalf("ExpiryGrace=%s want=%s", timing.ExpiryGrace, 5*time.Minute)
	}
}

func TestResolveDurationTiming_ForTwoDayPeriod(t *testing.T) {
	timing := Resolve(domain.Connector{
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: int64((48 * time.Hour) / time.Second),
	})

	if timing.ShortDuration {
		t.Fatal("ShortDuration=true want false")
	}
	if !timing.CustomRebillTiming {
		t.Fatal("CustomRebillTiming=false want true")
	}
	if timing.FirstRebillLead != 16*time.Hour {
		t.Fatalf("FirstRebillLead=%s want=%s", timing.FirstRebillLead, 16*time.Hour)
	}
	if timing.SecondRebillLead != 4*time.Hour+48*time.Minute {
		t.Fatalf("SecondRebillLead=%s want=%s", timing.SecondRebillLead, 4*time.Hour+48*time.Minute)
	}
	if timing.ThirdRebillLead != 2*time.Hour+24*time.Minute {
		t.Fatalf("ThirdRebillLead=%s want=%s", timing.ThirdRebillLead, 2*time.Hour+24*time.Minute)
	}
	if timing.ExpiryGrace != 0 {
		t.Fatalf("ExpiryGrace=%s want=0", timing.ExpiryGrace)
	}
}
