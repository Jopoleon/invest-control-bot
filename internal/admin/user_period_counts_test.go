package admin

import (
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func TestBuildUserPeriodCounts_SplitsCurrentAndNextByUser(t *testing.T) {
	now := time.Now().UTC()
	counts := buildUserPeriodCounts([]domain.Subscription{
		{
			UserID:   1,
			Status:   domain.SubscriptionStatusActive,
			StartsAt: now.Add(-time.Hour),
			EndsAt:   now.Add(time.Hour),
		},
		{
			UserID:   1,
			Status:   domain.SubscriptionStatusActive,
			StartsAt: now.Add(time.Hour),
			EndsAt:   now.Add(2 * time.Hour),
		},
		{
			UserID:   2,
			Status:   domain.SubscriptionStatusActive,
			StartsAt: now.Add(time.Hour),
			EndsAt:   now.Add(2 * time.Hour),
		},
		{
			UserID:   2,
			Status:   domain.SubscriptionStatusExpired,
			StartsAt: now.Add(-3 * time.Hour),
			EndsAt:   now.Add(-2 * time.Hour),
		},
	}, now)

	if counts[1] != (userPeriodCounts{Current: 1, Next: 1}) {
		t.Fatalf("counts[1] = %+v, want {Current:1 Next:1}", counts[1])
	}
	if counts[2] != (userPeriodCounts{Current: 0, Next: 1}) {
		t.Fatalf("counts[2] = %+v, want {Current:0 Next:1}", counts[2])
	}
}

func TestBuildUserAutopayStates_PrefersCurrentPeriodOverFutureRenewal(t *testing.T) {
	now := time.Now().UTC()
	states := buildUserAutopayStates([]domain.Subscription{
		{
			UserID:         1,
			Status:         domain.SubscriptionStatusActive,
			AutoPayEnabled: false,
			StartsAt:       now.Add(-time.Hour),
			EndsAt:         now.Add(time.Hour),
		},
		{
			UserID:         1,
			Status:         domain.SubscriptionStatusActive,
			AutoPayEnabled: true,
			StartsAt:       now.Add(time.Hour),
			EndsAt:         now.Add(2 * time.Hour),
		},
	}, now)

	if states[1] != (userAutopayState{Enabled: false, Configured: true}) {
		t.Fatalf("states[1] = %+v, want {Enabled:false Configured:true}", states[1])
	}
}

func TestBuildUserAutopayStates_FallsBackToNextPeriodWhenNoCurrentExists(t *testing.T) {
	now := time.Now().UTC()
	states := buildUserAutopayStates([]domain.Subscription{
		{
			UserID:         2,
			Status:         domain.SubscriptionStatusActive,
			AutoPayEnabled: true,
			StartsAt:       now.Add(time.Hour),
			EndsAt:         now.Add(2 * time.Hour),
		},
	}, now)

	if states[2] != (userAutopayState{Enabled: true, Configured: true}) {
		t.Fatalf("states[2] = %+v, want {Enabled:true Configured:true}", states[2])
	}
}

func TestSortUsersForOperationalView_PrefersCurrentThenNextPeriods(t *testing.T) {
	users := []userView{
		{UserID: 3, DisplayName: "Charlie", CurrentPeriods: 0, NextPeriods: 0},
		{UserID: 2, DisplayName: "Bravo", CurrentPeriods: 0, NextPeriods: 2},
		{UserID: 1, DisplayName: "Alpha", CurrentPeriods: 1, NextPeriods: 0},
	}

	sortUsersForOperationalView(users)

	got := []int64{users[0].UserID, users[1].UserID, users[2].UserID}
	want := []int64{1, 2, 3}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("sorted user ids = %v, want %v", got, want)
		}
	}
}
