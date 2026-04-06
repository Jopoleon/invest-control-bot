package admin

import (
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func subscriptionStatusBadgeAt(lang string, sub domain.Subscription, now time.Time) (string, string) {
	if sub.IsFutureActiveAt(now) {
		return t(lang, "badge.subscription.future"), "is-accent"
	}
	return subscriptionStatusBadge(lang, sub.Status)
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
