package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (s *Store) SaveUser(ctx context.Context, user domain.User) error {
	now := time.Now().UTC()
	if user.ID > 0 {
		_, err := s.db.ExecContext(ctx, `
			UPDATE users
			SET full_name = $2,
				phone = $3,
				email = $4,
				updated_at = $5
			WHERE id = $1
		`, user.ID, user.FullName, user.Phone, user.Email, now)
		return err
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (
			full_name, phone, email, updated_at
		) VALUES ($1,$2,$3,$4)
	`, user.FullName, user.Phone, user.Email, now)
	return err
}

// GetUserByID fetches user by internal user ID.
func (s *Store) GetUserByID(ctx context.Context, userID int64) (domain.User, bool, error) {
	var u domain.User
	err := s.db.QueryRowContext(ctx, `
		SELECT id, full_name, phone, email, created_at, updated_at
		FROM users
		WHERE id = $1
	`, userID).Scan(&u.ID, &u.FullName, &u.Phone, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return domain.User{}, false, nil
	}
	if err != nil {
		return domain.User{}, false, err
	}
	return u, true, nil
}

// GetUser fetches user by Telegram ID as a compatibility helper while callers
// are still migrating off telegram_id-based store APIs.
func (s *Store) GetUser(ctx context.Context, telegramID int64) (domain.User, bool, error) {
	if telegramID <= 0 {
		return domain.User{}, false, nil
	}
	return s.GetUserByMessenger(ctx, domain.MessengerKindTelegram, strconv.FormatInt(telegramID, 10))
}

// GetUserByMessenger resolves an existing user by external messenger identity without creating one.
func (s *Store) GetUserByMessenger(ctx context.Context, kind domain.MessengerKind, messengerUserID string) (domain.User, bool, error) {
	var u domain.User
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.full_name, u.phone, u.email, u.created_at, u.updated_at
		FROM user_messenger_accounts a
		JOIN users u ON u.id = a.user_id
		WHERE a.messenger_kind = $1 AND a.messenger_user_id = $2
	`, string(kind), messengerUserID).Scan(&u.ID, &u.FullName, &u.Phone, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return domain.User{}, false, nil
	}
	if err != nil {
		return domain.User{}, false, err
	}
	return u, true, nil
}

// GetOrCreateUserByMessenger resolves a user by external messenger identity and creates one if absent.
func (s *Store) GetOrCreateUserByMessenger(ctx context.Context, kind domain.MessengerKind, messengerUserID, username string) (domain.User, bool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return domain.User{}, false, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var existing domain.User
	err = tx.QueryRowContext(ctx, `
		SELECT u.id, u.full_name, u.phone, u.email, u.created_at, u.updated_at
		FROM user_messenger_accounts a
		JOIN users u ON u.id = a.user_id
		WHERE a.messenger_kind = $1 AND a.messenger_user_id = $2
	`, string(kind), messengerUserID).Scan(
		&existing.ID,
		&existing.FullName,
		&existing.Phone,
		&existing.Email,
		&existing.CreatedAt,
		&existing.UpdatedAt,
	)
	if err == nil {
		if strings.TrimSpace(username) != "" {
			if _, err := tx.ExecContext(ctx, `
				UPDATE user_messenger_accounts
				SET username = $3, updated_at = $4
				WHERE messenger_kind = $1 AND messenger_user_id = $2
			`, string(kind), messengerUserID, username, time.Now().UTC()); err != nil {
				return domain.User{}, false, err
			}
		}
		if err := tx.Commit(); err != nil {
			return domain.User{}, false, err
		}
		return existing, false, nil
	}
	if err != sql.ErrNoRows {
		return domain.User{}, false, err
	}

	var created domain.User
	err = tx.QueryRowContext(ctx, `
		INSERT INTO users (full_name, phone, email, updated_at)
		VALUES ('','','',$1)
		RETURNING id, full_name, phone, email, created_at, updated_at
	`, time.Now().UTC()).Scan(
		&created.ID,
		&created.FullName,
		&created.Phone,
		&created.Email,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return domain.User{}, false, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_messenger_accounts (
			user_id, messenger_kind, messenger_user_id, username, linked_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6)
	`, created.ID, string(kind), messengerUserID, username, time.Now().UTC(), time.Now().UTC()); err != nil {
		return domain.User{}, false, err
	}

	if err := tx.Commit(); err != nil {
		return domain.User{}, false, err
	}
	return created, true, nil
}

// ListUserMessengerAccounts returns linked external messenger identities for a user.
func (s *Store) ListUserMessengerAccounts(ctx context.Context, userID int64) ([]domain.UserMessengerAccount, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, messenger_kind, messenger_user_id, username, linked_at, updated_at
		FROM user_messenger_accounts
		WHERE user_id = $1
		ORDER BY messenger_kind ASC, messenger_user_id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.UserMessengerAccount, 0)
	for rows.Next() {
		var (
			item domain.UserMessengerAccount
			kind string
		)
		if err := rows.Scan(
			&item.UserID,
			&kind,
			&item.MessengerUserID,
			&item.Username,
			&item.LinkedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.MessengerKind = domain.MessengerKind(kind)
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListUsers returns admin-oriented user list with Telegram identity projection
// kept as a bridge while admin screens still render telegram-first columns.
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

	if query.UserID > 0 {
		where = append(where, fmt.Sprintf("u.id = $%d", argPos))
		args = append(args, query.UserID)
		argPos++
	}
	if query.TelegramID > 0 {
		where = append(where, fmt.Sprintf("ta.messenger_user_id = $%d", argPos))
		args = append(args, strconv.FormatInt(query.TelegramID, 10))
		argPos++
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		like := "%" + search + "%"
		where = append(where, fmt.Sprintf("(COALESCE(ta.messenger_user_id, '') ILIKE $%d OR COALESCE(ta.username, '') ILIKE $%d OR u.full_name ILIKE $%d OR u.phone ILIKE $%d OR u.email ILIKE $%d)", argPos, argPos, argPos, argPos, argPos))
		args = append(args, like)
		argPos++
	}

	stmt := `
		SELECT
			u.id,
			COALESCE(NULLIF(ta.messenger_user_id, ''), '0')::BIGINT AS telegram_id,
			COALESCE(ta.username, '') AS telegram_username,
			u.full_name,
			u.phone,
			u.email,
			COALESCE(sub_summary.auto_pay_enabled, false) AS auto_pay_enabled,
			COALESCE(sub_summary.has_active_subscriptions, false) AS has_auto_pay_settings,
			u.updated_at
		FROM users u
		LEFT JOIN user_messenger_accounts ta
			ON ta.user_id = u.id AND ta.messenger_kind = 'telegram'
		LEFT JOIN (
			SELECT
				user_id,
				BOOL_OR(auto_pay_enabled) FILTER (WHERE status = 'active') AS auto_pay_enabled,
				COUNT(*) FILTER (WHERE status = 'active') > 0 AS has_active_subscriptions
			FROM subscriptions
			GROUP BY user_id
		) sub_summary ON sub_summary.user_id = u.id
	`
	if len(where) > 0 {
		stmt += " WHERE " + strings.Join(where, " AND ")
	}
	stmt += fmt.Sprintf(" ORDER BY u.updated_at DESC, ta.messenger_user_id DESC NULLS LAST LIMIT $%d", argPos)
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
			&item.UserID,
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

func (s *Store) resolveUserIdentity(ctx context.Context, userID, telegramID int64) (int64, int64, error) {
	if userID > 0 {
		if _, found, err := s.GetUserByID(ctx, userID); err != nil {
			return 0, 0, err
		} else if found {
			if telegramID <= 0 {
				account, found, err := s.getMessengerAccount(ctx, userID, domain.MessengerKindTelegram)
				if err != nil {
					return 0, 0, err
				}
				if found {
					if parsedID, err := strconv.ParseInt(account.MessengerUserID, 10, 64); err == nil && parsedID > 0 {
						telegramID = parsedID
					}
				}
			}
			return userID, telegramID, nil
		}
	}
	if telegramID > 0 {
		user, found, err := s.GetUser(ctx, telegramID)
		if err != nil {
			return 0, 0, err
		}
		if found {
			return user.ID, telegramID, nil
		}
	}
	return userID, telegramID, nil
}

func (s *Store) getMessengerAccount(ctx context.Context, userID int64, kind domain.MessengerKind) (domain.UserMessengerAccount, bool, error) {
	var (
		item      domain.UserMessengerAccount
		kindValue string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, messenger_kind, messenger_user_id, username, linked_at, updated_at
		FROM user_messenger_accounts
		WHERE user_id = $1 AND messenger_kind = $2
	`, userID, string(kind)).Scan(
		&item.UserID,
		&kindValue,
		&item.MessengerUserID,
		&item.Username,
		&item.LinkedAt,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return domain.UserMessengerAccount{}, false, nil
	}
	if err != nil {
		return domain.UserMessengerAccount{}, false, err
	}
	item.MessengerKind = domain.MessengerKind(kindValue)
	return item, true, nil
}
