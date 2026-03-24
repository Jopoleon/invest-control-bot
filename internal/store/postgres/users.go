package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (s *Store) SaveUser(ctx context.Context, user domain.User) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (
			telegram_id, telegram_username, full_name, phone, email, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (telegram_id)
		DO UPDATE SET
			telegram_username = EXCLUDED.telegram_username,
			full_name = EXCLUDED.full_name,
			phone = EXCLUDED.phone,
			email = EXCLUDED.email,
			updated_at = EXCLUDED.updated_at
	`, user.TelegramID, user.TelegramUsername, user.FullName, user.Phone, user.Email, now)
	return err
}

// GetUser fetches user by Telegram ID.
func (s *Store) GetUser(ctx context.Context, telegramID int64) (domain.User, bool, error) {
	var u domain.User
	err := s.db.QueryRowContext(ctx, `
		SELECT telegram_id, telegram_username, full_name, phone, email, updated_at
		FROM users
		WHERE telegram_id = $1
	`, telegramID).Scan(&u.TelegramID, &u.TelegramUsername, &u.FullName, &u.Phone, &u.Email, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return domain.User{}, false, nil
	}
	if err != nil {
		return domain.User{}, false, err
	}
	return u, true, nil
}

// ListUsers returns admin-oriented user list with recurring preference metadata.
func (s *Store) ListUsers(ctx context.Context, query domain.UserListQuery) ([]domain.UserListItem, error) {
	if query.Limit <= 0 {
		query.Limit = 200
	}
	if query.Limit > 1000 {
		query.Limit = 1000
	}

	where := make([]string, 0, 2)
	args := make([]any, 0, 4)
	argPos := 1

	if query.TelegramID > 0 {
		where = append(where, fmt.Sprintf("u.telegram_id = $%d", argPos))
		args = append(args, query.TelegramID)
		argPos++
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		like := "%" + search + "%"
		where = append(where, fmt.Sprintf("(CAST(u.telegram_id AS TEXT) ILIKE $%d OR u.telegram_username ILIKE $%d OR u.full_name ILIKE $%d OR u.phone ILIKE $%d OR u.email ILIKE $%d)", argPos, argPos, argPos, argPos, argPos))
		args = append(args, like)
		argPos++
	}

	stmt := `
		SELECT
			u.telegram_id,
			u.telegram_username,
			u.full_name,
			u.phone,
			u.email,
			COALESCE(us.auto_pay_enabled, false) AS auto_pay_enabled,
			us.telegram_id IS NOT NULL AS has_auto_pay_settings,
			u.updated_at
		FROM users u
		LEFT JOIN user_settings us ON us.telegram_id = u.telegram_id
	`
	if len(where) > 0 {
		stmt += " WHERE " + strings.Join(where, " AND ")
	}
	stmt += fmt.Sprintf(" ORDER BY u.updated_at DESC, u.telegram_id DESC LIMIT $%d", argPos)
	args = append(args, query.Limit)

	rows, err := s.db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.UserListItem, 0)
	for rows.Next() {
		var item domain.UserListItem
		if err := rows.Scan(
			&item.TelegramID,
			&item.TelegramUsername,
			&item.FullName,
			&item.Phone,
			&item.Email,
			&item.AutoPayEnabled,
			&item.HasAutoPaySettings,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// SetUserAutoPayEnabled stores user recurring preference used for future payment creation.
func (s *Store) SetUserAutoPayEnabled(ctx context.Context, telegramID int64, enabled bool, updatedAt time.Time) error {
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_settings (telegram_id, auto_pay_enabled, updated_at)
		VALUES ($1,$2,$3)
		ON CONFLICT (telegram_id)
		DO UPDATE SET
			auto_pay_enabled = EXCLUDED.auto_pay_enabled,
			updated_at = EXCLUDED.updated_at
	`, telegramID, enabled, updatedAt)
	return err
}

// GetUserAutoPayEnabled returns user recurring preference if record exists.
func (s *Store) GetUserAutoPayEnabled(ctx context.Context, telegramID int64) (bool, bool, error) {
	var enabled bool
	err := s.db.QueryRowContext(ctx, `
		SELECT auto_pay_enabled
		FROM user_settings
		WHERE telegram_id = $1
	`, telegramID).Scan(&enabled)
	if err == sql.ErrNoRows {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return enabled, true, nil
}
