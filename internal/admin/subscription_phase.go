package admin

import (
	"sort"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func subscriptionStatusBadgeAt(lang string, sub domain.Subscription, now time.Time) (string, string) {
	if sub.IsCurrentActiveAt(now) {
		return t(lang, "badge.subscription.current"), "is-success"
	}
	if sub.IsFutureActiveAt(now) {
		return t(lang, "badge.subscription.future"), "is-accent"
	}
	return subscriptionStatusBadge(lang, sub.Status)
}

// selectAutopayScopeSubscriptions chooses the subscription slice that should
// drive operator-facing autopay state.
//
// The core rule is phase precedence, not "any active row":
//  1. Current active periods win.
//  2. If there is no current period, fall back to future active renewals.
//  3. Historical rows do not affect the current autopay badge.
//
// This keeps admin screens aligned with what the user is actually living
// through right now and avoids future renewals masking the current period.
func selectAutopayScopeSubscriptions(subs []domain.Subscription, now time.Time) []domain.Subscription {
	current := make([]domain.Subscription, 0, len(subs))
	future := make([]domain.Subscription, 0, len(subs))
	for _, sub := range subs {
		switch {
		case sub.IsCurrentActiveAt(now):
			current = append(current, sub)
		case sub.IsFutureActiveAt(now):
			future = append(future, sub)
		}
	}
	if len(current) > 0 {
		return current
	}
	if len(future) > 0 {
		return future
	}
	return nil
}

// selectOperationalSubscription returns the single subscription row that should
// represent a user+connector pair in operator tooling such as churn analysis.
//
// The selection is phase-aware:
//  1. Prefer the current active period.
//  2. Otherwise prefer the nearest upcoming renewal.
//  3. Only then fall back to the latest historical row.
//
// Without this phase-aware ordering, a future renewal can incorrectly replace
// the current period in admin status calculations, which leads to misleading
// autopay state and actions.
func selectOperationalSubscription(subs []domain.Subscription, now time.Time) (domain.Subscription, bool) {
	if len(subs) == 0 {
		return domain.Subscription{}, false
	}
	best := subs[0]
	for _, candidate := range subs[1:] {
		if moreRelevantSubscription(candidate, best, now) {
			best = candidate
		}
	}
	return best, true
}

func moreRelevantSubscription(candidate, current domain.Subscription, now time.Time) bool {
	candidateRank := subscriptionOperationalRank(candidate, now)
	currentRank := subscriptionOperationalRank(current, now)
	if candidateRank != currentRank {
		return candidateRank > currentRank
	}

	switch candidateRank {
	case 3:
		if !candidate.EndsAt.Equal(current.EndsAt) {
			return candidate.EndsAt.After(current.EndsAt)
		}
	case 2:
		if !candidate.StartsAt.Equal(current.StartsAt) {
			return candidate.StartsAt.Before(current.StartsAt)
		}
	default:
		if !candidate.UpdatedAt.Equal(current.UpdatedAt) {
			return candidate.UpdatedAt.After(current.UpdatedAt)
		}
		if !candidate.EndsAt.Equal(current.EndsAt) {
			return candidate.EndsAt.After(current.EndsAt)
		}
	}

	if !candidate.CreatedAt.Equal(current.CreatedAt) {
		return candidate.CreatedAt.After(current.CreatedAt)
	}
	return candidate.ID > current.ID
}

func subscriptionOperationalRank(sub domain.Subscription, now time.Time) int {
	switch {
	case sub.IsCurrentActiveAt(now):
		return 3
	case sub.IsFutureActiveAt(now):
		return 2
	default:
		return 1
	}
}

// sortSubscriptionsForOperationalView orders subscriptions the way an operator
// needs to scan them in billing and user-detail tables:
//  1. Current active periods first.
//  2. Then upcoming renewal periods.
//  3. Then historical rows.
//
// Inside each phase we bias toward actionability:
//   - current periods ending sooner first
//   - future periods starting sooner first
//   - historical rows newer first
func sortSubscriptionsForOperationalView(subs []domain.Subscription, now time.Time) {
	sort.SliceStable(subs, func(i, j int) bool {
		left := subscriptionOperationalRank(subs[i], now)
		right := subscriptionOperationalRank(subs[j], now)
		if left != right {
			return left > right
		}

		switch left {
		case 3:
			if !subs[i].EndsAt.Equal(subs[j].EndsAt) {
				return subs[i].EndsAt.Before(subs[j].EndsAt)
			}
		case 2:
			if !subs[i].StartsAt.Equal(subs[j].StartsAt) {
				return subs[i].StartsAt.Before(subs[j].StartsAt)
			}
		default:
			if !subs[i].UpdatedAt.Equal(subs[j].UpdatedAt) {
				return subs[i].UpdatedAt.After(subs[j].UpdatedAt)
			}
		}

		if !subs[i].CreatedAt.Equal(subs[j].CreatedAt) {
			return subs[i].CreatedAt.After(subs[j].CreatedAt)
		}
		return subs[i].ID > subs[j].ID
	})
}

func countCurrentActiveSubscriptions(subs []domain.Subscription, now time.Time) int {
	count := 0
	for _, sub := range subs {
		if sub.IsCurrentActiveAt(now) {
			count++
		}
	}
	return count
}

func countFutureActiveSubscriptions(subs []domain.Subscription, now time.Time) int {
	count := 0
	for _, sub := range subs {
		if sub.IsFutureActiveAt(now) {
			count++
		}
	}
	return count
}

func subscriptionPhaseLabel(sub domain.Subscription, now time.Time) string {
	switch {
	case sub.IsCurrentActiveAt(now):
		return "current"
	case sub.IsFutureActiveAt(now):
		return "next"
	default:
		return "historical"
	}
}
