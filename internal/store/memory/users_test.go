package memory

import (
	"context"
	"testing"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func TestGetOrCreateUserByMessengerCreatesTelegramUserAndAccount(t *testing.T) {
	st := New()
	ctx := context.Background()

	user, created, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "264704572", "emiloserdov")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	if !created {
		t.Fatalf("created = false, want true")
	}
	if user.ID == 0 {
		t.Fatalf("user ID = 0")
	}

	gotByTelegram, found, err := st.GetUser(ctx, 264704572)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if !found || gotByTelegram.ID != user.ID {
		t.Fatalf("GetUser returned %+v, found=%v", gotByTelegram, found)
	}

	gotByID, found, err := st.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if !found || gotByID.ID != user.ID {
		t.Fatalf("GetUserByID returned %+v, found=%v", gotByID, found)
	}

	accounts, err := st.ListUserMessengerAccounts(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListUserMessengerAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("accounts len = %d, want 1", len(accounts))
	}
	if accounts[0].MessengerKind != domain.MessengerKindTelegram || accounts[0].MessengerUserID != "264704572" {
		t.Fatalf("unexpected account: %+v", accounts[0])
	}

	gotByMessenger, found, err := st.GetUserByMessenger(ctx, domain.MessengerKindTelegram, "264704572")
	if err != nil {
		t.Fatalf("GetUserByMessenger: %v", err)
	}
	if !found || gotByMessenger.ID != user.ID {
		t.Fatalf("GetUserByMessenger returned %+v, found=%v", gotByMessenger, found)
	}
}

func TestGetOrCreateUserByMessengerReturnsExistingAndRefreshesUsername(t *testing.T) {
	st := New()
	ctx := context.Background()

	createdUser, created, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "max-user-1", "old_name")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger create: %v", err)
	}
	if !created {
		t.Fatalf("created = false, want true")
	}

	existingUser, created, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "max-user-1", "new_name")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger existing: %v", err)
	}
	if created {
		t.Fatalf("created = true, want false")
	}
	if existingUser.ID != createdUser.ID {
		t.Fatalf("existing user id = %d, want %d", existingUser.ID, createdUser.ID)
	}

	accounts, err := st.ListUserMessengerAccounts(ctx, createdUser.ID)
	if err != nil {
		t.Fatalf("ListUserMessengerAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("accounts len = %d, want 1", len(accounts))
	}
	if accounts[0].Username != "new_name" {
		t.Fatalf("username = %q, want %q", accounts[0].Username, "new_name")
	}
}
