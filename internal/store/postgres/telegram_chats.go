package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (s *Store) UpsertTelegramChat(ctx context.Context, chat domain.TelegramChat) error {
	now := time.Now().UTC()
	if chat.LastSeenAt.IsZero() {
		chat.LastSeenAt = now
	}
	if chat.UpdatedAt.IsZero() {
		chat.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
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
	`,
		chat.ChatID,
		chat.Title,
		chat.Username,
		chat.Type,
		chat.BotStatus,
		chat.CanInviteUsers,
		chat.CanRestrictMembers,
		chat.CanManageChat,
		chat.LastSeenAt,
		chat.LastVerifiedAt,
		chat.UpdatedAt,
	)
	return err
}

func (s *Store) ListTelegramChats(ctx context.Context) ([]domain.TelegramChat, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT chat_id, title, username, chat_type, bot_status,
		       can_invite_users, can_restrict_members, can_manage_chat,
		       last_seen_at, last_verified_at, updated_at
		FROM telegram_chats
		ORDER BY
			CASE WHEN bot_status = 'administrator' THEN 0 ELSE 1 END,
			lower(title),
			chat_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.TelegramChat, 0, 16)
	for rows.Next() {
		item, err := scanTelegramChat(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetTelegramChat(ctx context.Context, chatID string) (domain.TelegramChat, bool, error) {
	item, err := scanTelegramChat(s.db.QueryRowContext(ctx, `
		SELECT chat_id, title, username, chat_type, bot_status,
		       can_invite_users, can_restrict_members, can_manage_chat,
		       last_seen_at, last_verified_at, updated_at
		FROM telegram_chats
		WHERE chat_id = $1
	`, chatID))
	if err == sql.ErrNoRows {
		return domain.TelegramChat{}, false, nil
	}
	if err != nil {
		return domain.TelegramChat{}, false, err
	}
	return item, true, nil
}
