package admin

import (
	"sort"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

type userPeriodCounts struct {
	Current int
	Next    int
}

type userAutopayState struct {
	Enabled    bool
	Configured bool
}

func buildUserPeriodCounts(subs []domain.Subscription, now time.Time) map[int64]userPeriodCounts {
	counts := make(map[int64]userPeriodCounts)
	for _, sub := range subs {
		item := counts[sub.UserID]
		switch {
		case sub.IsCurrentActiveAt(now):
			item.Current++
		case sub.IsFutureActiveAt(now):
			item.Next++
		default:
			continue
		}
		counts[sub.UserID] = item
	}
	return counts
}

func buildUserAutopayStates(subs []domain.Subscription, now time.Time) map[int64]userAutopayState {
	grouped := make(map[int64][]domain.Subscription)
	for _, sub := range subs {
		grouped[sub.UserID] = append(grouped[sub.UserID], sub)
	}
	states := make(map[int64]userAutopayState, len(grouped))
	for userID, userSubs := range grouped {
		enabled, configured := summarizeAutopayFromSubscriptions(userSubs, now)
		states[userID] = userAutopayState{
			Enabled:    enabled,
			Configured: configured,
		}
	}
	return states
}

func sortUsersForOperationalView(users []userView) {
	sort.SliceStable(users, func(i, j int) bool {
		leftCurrent := users[i].CurrentPeriods
		rightCurrent := users[j].CurrentPeriods
		if leftCurrent != rightCurrent {
			return leftCurrent > rightCurrent
		}
		leftNext := users[i].NextPeriods
		rightNext := users[j].NextPeriods
		if leftNext != rightNext {
			return leftNext > rightNext
		}
		if users[i].DisplayName != users[j].DisplayName {
			return users[i].DisplayName < users[j].DisplayName
		}
		return users[i].UserID < users[j].UserID
	})
}
