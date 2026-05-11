package chatstatus

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/max"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
)

type fakeTelegramChecker struct {
	info telegram.ChatMemberInfo
	err  error
}

func (f fakeTelegramChecker) GetChatMember(context.Context, string, int64) (telegram.ChatMemberInfo, error) {
	return f.info, f.err
}

type fakeMAXChecker struct {
	page max.ChatMembersPage
	err  error
}

func (f fakeMAXChecker) GetChatMembers(context.Context, int64, []int64) (max.ChatMembersPage, error) {
	return f.page, f.err
}

func TestRefreshTelegramStoresBannedStatusAndRawCheck(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	svc := Service{
		Store: st,
		Telegram: fakeTelegramChecker{info: telegram.ChatMemberInfo{
			Status: "kicked",
			UserID: 5228612359,
			Raw:    []byte(`{"status":"kicked","user":{"id":5228612359}}`),
		}},
		Now: func() time.Time { return now },
	}

	state, err := svc.Refresh(ctx, RefreshRequest{
		MessengerKind:   domain.MessengerKindTelegram,
		ChatRef:         "-1003222018503",
		MessengerUserID: "5228612359",
		UserID:          14,
		ConnectorID:     20,
		SubscriptionID:  76,
		Source:          "admin_refresh",
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if state.Status != domain.MessengerChatUserStatusBanned || !state.IsBanned || state.IsMember {
		t.Fatalf("state=%+v want banned non-member", state)
	}
	saved, found, err := st.GetMessengerChatUser(ctx, domain.MessengerKindTelegram, "-1003222018503", "5228612359")
	if err != nil || !found {
		t.Fatalf("GetMessengerChatUser found=%v err=%v", found, err)
	}
	if saved.Status != domain.MessengerChatUserStatusBanned || saved.LastKickedAt == nil {
		t.Fatalf("saved=%+v want kicked timestamp", saved)
	}
	checks, err := st.ListMessengerChatUserChecks(ctx, domain.MessengerKindTelegram, "-1003222018503", "5228612359", 10)
	if err != nil {
		t.Fatalf("ListMessengerChatUserChecks: %v", err)
	}
	if len(checks) != 1 || !checks[0].OK || string(checks[0].RawJSON) == "" {
		t.Fatalf("checks=%+v want one successful raw check", checks)
	}
}

func TestRefreshMAXStoresNotMemberWhenUserMissingFromResponse(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 5, 9, 11, 0, 0, 0, time.UTC)
	svc := Service{
		Store: st,
		MAX: fakeMAXChecker{page: max.ChatMembersPage{
			Members: nil,
			Raw:     []byte(`{"members":[]}`),
		}},
		Now: func() time.Time { return now },
	}

	state, err := svc.Refresh(ctx, RefreshRequest{
		MessengerKind:   domain.MessengerKindMAX,
		ChatRef:         "-72598909498032",
		MessengerUserID: "193465776",
		UserID:          7,
		Source:          "admin_refresh",
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if state.Status != domain.MessengerChatUserStatusNotMember || state.IsMember || state.LastLeftAt == nil {
		t.Fatalf("state=%+v want not_member", state)
	}
}

func TestRefreshMAXStoresAdministratorPermissions(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	svc := Service{
		Store: st,
		MAX: fakeMAXChecker{page: max.ChatMembersPage{
			Members: []max.ChatMember{{
				UserID:      193465776,
				IsAdmin:     true,
				Permissions: []string{"add_remove_members"},
			}},
			Raw: []byte(`{"members":[{"user_id":193465776,"is_admin":true,"permissions":["add_remove_members"]}]}`),
		}},
		Now: func() time.Time { return time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC) },
	}

	state, err := svc.Refresh(ctx, RefreshRequest{
		MessengerKind:   domain.MessengerKindMAX,
		ChatRef:         "-72598909498032",
		MessengerUserID: "193465776",
		UserID:          7,
		Source:          "admin_refresh",
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if state.Status != domain.MessengerChatUserStatusAdministrator || !state.IsAdmin || !state.IsMember {
		t.Fatalf("state=%+v want administrator member", state)
	}
	if len(state.Permissions) != 1 || state.Permissions[0] != "add_remove_members" {
		t.Fatalf("permissions=%v", state.Permissions)
	}
}

func TestRefreshPersistsErrorStateAndReturnsProviderError(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	providerErr := errors.New("telegram api failed")
	svc := Service{
		Store:    st,
		Telegram: fakeTelegramChecker{err: providerErr},
		Now:      func() time.Time { return time.Date(2026, 5, 9, 13, 0, 0, 0, time.UTC) },
	}

	state, err := svc.Refresh(ctx, RefreshRequest{
		MessengerKind:   domain.MessengerKindTelegram,
		ChatRef:         "-1003222018503",
		MessengerUserID: "5228612359",
		UserID:          14,
		Source:          "admin_refresh",
	})
	if !errors.Is(err, providerErr) {
		t.Fatalf("err=%v want provider err", err)
	}
	if state.Status != domain.MessengerChatUserStatusError || state.LastError != providerErr.Error() {
		t.Fatalf("state=%+v want error state", state)
	}
	saved, found, err := st.GetMessengerChatUser(ctx, domain.MessengerKindTelegram, "-1003222018503", "5228612359")
	if err != nil || !found {
		t.Fatalf("GetMessengerChatUser found=%v err=%v", found, err)
	}
	if saved.Status != domain.MessengerChatUserStatusError {
		t.Fatalf("saved=%+v want error", saved)
	}
}
