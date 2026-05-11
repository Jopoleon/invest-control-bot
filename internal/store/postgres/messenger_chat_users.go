package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (s *Store) UpsertMessengerChatUser(ctx context.Context, user domain.MessengerChatUser) error {
	permissions, err := json.Marshal(user.Permissions)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO messenger_chat_users (
			messenger_kind, chat_ref, messenger_user_id, user_id, status,
			is_member, is_banned, is_restricted, is_admin, is_owner,
			permissions, checked_at, last_joined_at, last_left_at, last_kicked_at,
			last_unbanned_at, last_error, updated_at
		) VALUES (
			$1, $2, $3, NULLIF($4, 0), $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15,
			$16, $17, $18
		)
		ON CONFLICT (messenger_kind, chat_ref, messenger_user_id) DO UPDATE SET
			user_id = COALESCE(EXCLUDED.user_id, messenger_chat_users.user_id),
			status = EXCLUDED.status,
			is_member = EXCLUDED.is_member,
			is_banned = EXCLUDED.is_banned,
			is_restricted = EXCLUDED.is_restricted,
			is_admin = EXCLUDED.is_admin,
			is_owner = EXCLUDED.is_owner,
			permissions = EXCLUDED.permissions,
			checked_at = EXCLUDED.checked_at,
			last_joined_at = COALESCE(EXCLUDED.last_joined_at, messenger_chat_users.last_joined_at),
			last_left_at = COALESCE(EXCLUDED.last_left_at, messenger_chat_users.last_left_at),
			last_kicked_at = COALESCE(EXCLUDED.last_kicked_at, messenger_chat_users.last_kicked_at),
			last_unbanned_at = COALESCE(EXCLUDED.last_unbanned_at, messenger_chat_users.last_unbanned_at),
			last_error = EXCLUDED.last_error,
			updated_at = EXCLUDED.updated_at
	`, string(user.MessengerKind), user.ChatRef, user.MessengerUserID, user.UserID, string(user.Status),
		user.IsMember, user.IsBanned, user.IsRestricted, user.IsAdmin, user.IsOwner,
		permissions, user.CheckedAt, nullableTime(user.LastJoinedAt), nullableTime(user.LastLeftAt), nullableTime(user.LastKickedAt),
		nullableTime(user.LastUnbannedAt), user.LastError, user.UpdatedAt)
	return err
}

func (s *Store) GetMessengerChatUser(ctx context.Context, kind domain.MessengerKind, chatRef, messengerUserID string) (domain.MessengerChatUser, bool, error) {
	item, err := scanMessengerChatUser(s.db.QueryRowContext(ctx, messengerChatUserSelect()+`
		WHERE messenger_kind = $1 AND chat_ref = $2 AND messenger_user_id = $3
	`, string(kind), chatRef, messengerUserID))
	if err == sql.ErrNoRows {
		return domain.MessengerChatUser{}, false, nil
	}
	if err != nil {
		return domain.MessengerChatUser{}, false, err
	}
	return item, true, nil
}

func (s *Store) ListMessengerChatUsersByUser(ctx context.Context, userID int64) ([]domain.MessengerChatUser, error) {
	rows, err := s.db.QueryContext(ctx, messengerChatUserSelect()+`
		WHERE user_id = $1
		ORDER BY updated_at DESC, messenger_kind ASC, chat_ref ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.MessengerChatUser, 0, 8)
	for rows.Next() {
		item, err := scanMessengerChatUser(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) SaveMessengerChatUserCheck(ctx context.Context, check domain.MessengerChatUserCheck) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO messenger_chat_user_checks (
			messenger_kind, chat_ref, messenger_user_id, user_id, connector_id,
			subscription_id, source, status, ok, raw_json, raw_error, checked_at
		) VALUES (
			$1, $2, $3, NULLIF($4, 0), NULLIF($5, 0),
			NULLIF($6, 0), $7, $8, $9, $10, $11, $12
		)
	`, string(check.MessengerKind), check.ChatRef, check.MessengerUserID, check.UserID, check.ConnectorID,
		check.SubscriptionID, check.Source, string(check.Status), check.OK, nullableJSON(check.RawJSON), check.RawError, check.CheckedAt)
	return err
}

func (s *Store) ListMessengerChatUserChecks(ctx context.Context, kind domain.MessengerKind, chatRef, messengerUserID string, limit int) ([]domain.MessengerChatUserCheck, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, messenger_kind, chat_ref, messenger_user_id, user_id, connector_id,
			subscription_id, source, status, ok, raw_json, raw_error, checked_at
		FROM messenger_chat_user_checks
		WHERE messenger_kind = $1 AND chat_ref = $2 AND messenger_user_id = $3
		ORDER BY checked_at DESC, id DESC
		LIMIT $4
	`, string(kind), chatRef, messengerUserID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.MessengerChatUserCheck, 0, limit)
	for rows.Next() {
		item, err := scanMessengerChatUserCheck(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func messengerChatUserSelect() string {
	return `SELECT messenger_kind, chat_ref, messenger_user_id, user_id, status,
		is_member, is_banned, is_restricted, is_admin, is_owner,
		permissions, checked_at, last_joined_at, last_left_at, last_kicked_at,
		last_unbanned_at, last_error, updated_at
		FROM messenger_chat_users `
}

func nullableTime(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC()
}

func nullableJSON(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}
