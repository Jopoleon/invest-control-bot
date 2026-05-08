package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func TestUpsertTelegramChat(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	seenAt := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`
		INSERT INTO telegram_chats (
			chat_id, title, username, chat_type, bot_status,
			can_invite_users, can_restrict_members, can_manage_chat,
			last_seen_at, last_verified_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (chat_id) DO UPDATE SET
			title = EXCLUDED.title,
			username = EXCLUDED.username,
			chat_type = EXCLUDED.chat_type,
			bot_status = EXCLUDED.bot_status,
			can_invite_users = EXCLUDED.can_invite_users,
			can_restrict_members = EXCLUDED.can_restrict_members,
			can_manage_chat = EXCLUDED.can_manage_chat,
			last_seen_at = EXCLUDED.last_seen_at,
			last_verified_at = COALESCE(EXCLUDED.last_verified_at, telegram_chats.last_verified_at),
			updated_at = EXCLUDED.updated_at
	`)).
		WithArgs("-100123", "Paid Chat", "paid_chat", "supergroup", "administrator", true, true, true, seenAt, nil, seenAt).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.UpsertTelegramChat(context.Background(), domain.TelegramChat{
		ChatID:             "-100123",
		Title:              "Paid Chat",
		Username:           "paid_chat",
		Type:               "supergroup",
		BotStatus:          "administrator",
		CanInviteUsers:     true,
		CanRestrictMembers: true,
		CanManageChat:      true,
		LastSeenAt:         seenAt,
		UpdatedAt:          seenAt,
	}); err != nil {
		t.Fatalf("UpsertTelegramChat: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestListTelegramChatsOrdersAdminFirst(t *testing.T) {
	store, mock, cleanup := newMockStore(t)
	defer cleanup()

	now := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"chat_id", "title", "username", "chat_type", "bot_status",
		"can_invite_users", "can_restrict_members", "can_manage_chat",
		"last_seen_at", "last_verified_at", "updated_at",
	}).AddRow("-1001", "A", "", "channel", "administrator", true, true, true, now, nil, now)
	mock.ExpectQuery(regexp.QuoteMeta(`
		SELECT chat_id, title, username, chat_type, bot_status,
		       can_invite_users, can_restrict_members, can_manage_chat,
		       last_seen_at, last_verified_at, updated_at
		FROM telegram_chats
		ORDER BY
			CASE WHEN bot_status = 'administrator' THEN 0 ELSE 1 END,
			lower(title),
			chat_id
	`)).WillReturnRows(rows)

	chats, err := store.ListTelegramChats(context.Background())
	if err != nil {
		t.Fatalf("ListTelegramChats: %v", err)
	}
	if len(chats) != 1 || chats[0].ChatID != "-1001" || !chats[0].CanInviteUsers {
		t.Fatalf("chats=%+v want admin chat", chats)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
