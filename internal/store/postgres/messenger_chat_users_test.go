package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func TestUpsertMessengerChatUser(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO messenger_chat_users (`)).
		WithArgs(
			string(domain.MessengerKindTelegram), "-1003222018503", "5228612359", int64(14), string(domain.MessengerChatUserStatusBanned),
			false, true, false, false, false,
			sqlmock.AnyArg(), now, nil, nil, sqlmock.AnyArg(),
			nil, "", now,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := store.UpsertMessengerChatUser(context.Background(), domain.MessengerChatUser{
		MessengerKind:   domain.MessengerKindTelegram,
		ChatRef:         "-1003222018503",
		MessengerUserID: "5228612359",
		UserID:          14,
		Status:          domain.MessengerChatUserStatusBanned,
		IsBanned:        true,
		CheckedAt:       now,
		LastKickedAt:    &now,
		UpdatedAt:       now,
	})
	if err != nil {
		t.Fatalf("UpsertMessengerChatUser: %v", err)
	}
}

func TestGetMessengerChatUser(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"messenger_kind", "chat_ref", "messenger_user_id", "user_id", "status",
		"is_member", "is_banned", "is_restricted", "is_admin", "is_owner",
		"permissions", "checked_at", "last_joined_at", "last_left_at", "last_kicked_at",
		"last_unbanned_at", "last_error", "updated_at",
	}).AddRow(
		string(domain.MessengerKindMAX), "-72598909498032", "193465776", int64(7), string(domain.MessengerChatUserStatusAdministrator),
		true, false, false, true, false,
		[]byte(`["add_remove_members"]`), now, now, nil, nil,
		nil, "", now,
	)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT messenger_kind, chat_ref, messenger_user_id, user_id, status,`)).
		WithArgs(string(domain.MessengerKindMAX), "-72598909498032", "193465776").
		WillReturnRows(rows)

	item, found, err := store.GetMessengerChatUser(context.Background(), domain.MessengerKindMAX, "-72598909498032", "193465776")
	if err != nil || !found {
		t.Fatalf("GetMessengerChatUser found=%v err=%v", found, err)
	}
	if item.Status != domain.MessengerChatUserStatusAdministrator || !item.IsAdmin || len(item.Permissions) != 1 || item.Permissions[0] != "add_remove_members" {
		t.Fatalf("item=%+v", item)
	}
}

func TestSaveAndListMessengerChatUserChecks(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	now := time.Date(2026, 5, 9, 10, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO messenger_chat_user_checks (`)).
		WithArgs(
			string(domain.MessengerKindTelegram), "-1003222018503", "5228612359", int64(14), int64(20),
			int64(76), "admin_refresh", string(domain.MessengerChatUserStatusBanned), true, sqlmock.AnyArg(), "", now,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.SaveMessengerChatUserCheck(context.Background(), domain.MessengerChatUserCheck{
		MessengerKind:   domain.MessengerKindTelegram,
		ChatRef:         "-1003222018503",
		MessengerUserID: "5228612359",
		UserID:          14,
		ConnectorID:     20,
		SubscriptionID:  76,
		Source:          "admin_refresh",
		Status:          domain.MessengerChatUserStatusBanned,
		OK:              true,
		RawJSON:         []byte(`{"status":"kicked"}`),
		CheckedAt:       now,
	})
	if err != nil {
		t.Fatalf("SaveMessengerChatUserCheck: %v", err)
	}

	rows := sqlmock.NewRows([]string{
		"id", "messenger_kind", "chat_ref", "messenger_user_id", "user_id", "connector_id",
		"subscription_id", "source", "status", "ok", "raw_json", "raw_error", "checked_at",
	}).AddRow(
		int64(1), string(domain.MessengerKindTelegram), "-1003222018503", "5228612359", int64(14), int64(20),
		int64(76), "admin_refresh", string(domain.MessengerChatUserStatusBanned), true, []byte(`{"status":"kicked"}`), "", now,
	)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, messenger_kind, chat_ref, messenger_user_id, user_id, connector_id,`)).
		WithArgs(string(domain.MessengerKindTelegram), "-1003222018503", "5228612359", 10).
		WillReturnRows(rows)

	checks, err := store.ListMessengerChatUserChecks(context.Background(), domain.MessengerKindTelegram, "-1003222018503", "5228612359", 10)
	if err != nil {
		t.Fatalf("ListMessengerChatUserChecks: %v", err)
	}
	if len(checks) != 1 || string(checks[0].RawJSON) != `{"status":"kicked"}` {
		t.Fatalf("checks=%+v", checks)
	}
}
