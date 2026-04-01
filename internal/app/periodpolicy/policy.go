package periodpolicy

import (
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

const shortRecurringPeriodThreshold = 10 * time.Minute

type Timing struct {
	ShortDuration    bool
	FirstRebillLead  time.Duration
	SecondRebillLead time.Duration
	ExpiryGrace      time.Duration
}

// Resolve derives lifecycle and recurring timing from the connector period.
//
// Connectors with very short duration exist primarily for live-money recurring
// smoke tests. They need distinct rebill/expiry semantics so the scheduler
// does not behave like the monthly 72h/48h/24h policy on a 3-4 minute period.
//
// TODO: Revisit these short-duration windows after repeated live-money smoke
// tests in production. The current policy is intentionally explicit and
// test-friendly, but it should remain grounded in observed provider latency.
func Resolve(connector domain.Connector) Timing {
	duration, ok := connector.DurationPeriod()
	if !ok || duration <= 0 || duration > shortRecurringPeriodThreshold {
		return Timing{}
	}

	// First attempt should leave enough room for provider callback latency while
	// still avoiding an almost-immediate rebill right after the first payment.
	firstLead := clampDuration(duration/3, 30*time.Second, 90*time.Second)
	if maxLead := duration / 2; maxLead > 0 && firstLead > maxLead {
		firstLead = maxLead
	}

	// Second attempt stays close to the end of the period without collapsing
	// into the exact expiry boundary.
	secondLead := clampDuration(duration/10, 15*time.Second, 30*time.Second)
	if secondLead >= firstLead {
		secondLead = maxDuration(10*time.Second, firstLead/2)
	}

	// A short grace prevents false expiry/revoke when the provider result lands
	// a few seconds after the rebill request on smoke-test durations.
	expiryGrace := clampDuration(duration/6, 20*time.Second, 60*time.Second)

	return Timing{
		ShortDuration:    true,
		FirstRebillLead:  firstLead,
		SecondRebillLead: secondLead,
		ExpiryGrace:      expiryGrace,
	}
}

func (t Timing) RebillAttemptOrdinal(now, endsAt time.Time) int {
	if !t.ShortDuration {
		return 0
	}
	remaining := endsAt.Sub(now)
	switch {
	case remaining <= 0:
		return 0
	case remaining <= t.SecondRebillLead:
		return 2
	case remaining <= t.FirstRebillLead:
		return 1
	default:
		return 0
	}
}

func (t Timing) SuppressPreExpiryNotifications() bool {
	return t.ShortDuration
}

func (t Timing) ShouldDeferExpiration(now, endsAt time.Time) bool {
	if !t.ShortDuration || t.ExpiryGrace <= 0 {
		return false
	}
	return now.Before(endsAt.Add(t.ExpiryGrace))
}

func clampDuration(value, minValue, maxValue time.Duration) time.Duration {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
