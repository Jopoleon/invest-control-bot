package periodpolicy

import (
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

const (
	// Sub-day durations should suppress reminder/expiry notice jobs and tolerate
	// a short provider-callback grace before access is revoked.
	shortDurationThreshold = 24 * time.Hour
)

type Timing struct {
	ShortDuration      bool
	CustomRebillTiming bool
	FirstRebillLead    time.Duration
	SecondRebillLead   time.Duration
	ThirdRebillLead    time.Duration
	ExpiryGrace        time.Duration
}

// Resolve derives lifecycle and recurring timing from the connector period.
//
// Duration-based connectors use proportional rebill windows derived from the
// actual period length. This avoids applying the legacy monthly 72h/48h/24h
// schedule to short periods like 3 hours or 2 days.
//
// TODO: Revisit these derived windows after repeated production validation.
// They are intentionally proportional to the subscription duration so every
// duration-based connector follows the same rebill model.
func Resolve(connector domain.Connector) Timing {
	duration, ok := connector.DurationPeriod()
	if !ok || duration <= 0 {
		return Timing{}
	}
	return resolveDurationTiming(duration)
}

func resolveDurationTiming(duration time.Duration) Timing {
	firstLead := clampDuration(duration/3, 30*time.Second, 72*time.Hour)
	secondLead := clampDuration(duration/10, 15*time.Second, 48*time.Hour)
	thirdLead := clampDuration(duration/20, 5*time.Second, 24*time.Hour)
	if secondLead >= firstLead {
		secondLead = maxDuration(15*time.Second, firstLead/2)
	}
	if thirdLead >= secondLead {
		thirdLead = maxDuration(5*time.Second, secondLead/2)
	}

	shortDuration := duration <= shortDurationThreshold
	expiryGrace := time.Duration(0)
	if shortDuration {
		expiryGrace = clampDuration(duration/6, 20*time.Second, 5*time.Minute)
	}

	return Timing{
		ShortDuration:      shortDuration,
		CustomRebillTiming: true,
		FirstRebillLead:    firstLead,
		SecondRebillLead:   secondLead,
		ThirdRebillLead:    thirdLead,
		ExpiryGrace:        expiryGrace,
	}
}

func (t Timing) RebillAttemptOrdinal(now, endsAt time.Time) int {
	if !t.CustomRebillTiming {
		return 0
	}
	remaining := endsAt.Sub(now)
	switch {
	case remaining <= 0:
		return 0
	case t.ThirdRebillLead > 0 && remaining <= t.ThirdRebillLead:
		return 3
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

// LongPeriodPendingRebillStaleAfter is how long a pending rebill of an ordinary
// (non short-period) connector may wait for the Robokassa result callback
// before it is treated as stale.
//
// Robokassa answers the recurring request with OK+InvoiceID even when the
// actual charge is later declined, and it sends no failure callback in that
// case. Successful production rebills receive the result callback within
// seconds, so an hour without callback is a reliable "declined or lost" signal.
const LongPeriodPendingRebillStaleAfter = time.Hour

// IsPendingRebillStale reports whether a pending rebill created at
// pendingCreatedAt for a subscription ending at endsAt has waited too long for
// the provider result callback.
//
// Short-period connectors fire the rebill seconds before expiry, so their
// rebill is stale only after the subscription end, inside the callback grace
// window. Long-period connectors are judged by pending age while the
// subscription is still active, so the user and operators can react before
// access is revoked.
//
// A stale rebill is not a failed payment: a late success callback would still
// be honored. Callers use this only to surface the problem and to let the user
// renew manually.
func (t Timing) IsPendingRebillStale(now, pendingCreatedAt, endsAt time.Time) bool {
	if t.ShortDuration {
		return t.ShouldDeferExpiration(now, endsAt) && !now.Before(endsAt)
	}
	return now.Sub(pendingCreatedAt) >= LongPeriodPendingRebillStaleAfter
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
