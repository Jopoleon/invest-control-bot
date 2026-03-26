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
			INSERT INTO users (
				id, telegram_id, telegram_username, full_name, phone, email, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (id)
			DO UPDATE SET
				telegram_id = EXCLUDED.telegram_id,
				telegram_username = EXCLUDED.telegram_username,
				full_name = EXCLUDED.full_name,
				phone = EXCLUDED.phone,
				email = EXCLUDED.email,
				updated_at = EXCLUDED.updated_at
		`, user.ID, nullableTelegramID(user.TelegramID), user.TelegramUsername, user.FullName, user.Phone, user.Email, now)
		if err != nil {
			return err
		}
		if user.TelegramID > 0 {
			return s.saveUserMessengerAccount(ctx, domain.UserMessengerAccount{
				UserID:         user.ID,
				MessengerKind:  domain.MessengerKindTelegram,
				ExternalUserID: strconv.FormatInt(user.TelegramID, 10),
				Username:       user.TelegramUsername,
				LinkedAt:       now,
				UpdatedAt:      now,
			})
		}
		return nil
	}

	if user.TelegramID <= 0 {
		return fmt.Errorf("save user requires internal id or telegram_id")
	}
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
	if err != nil {
		return err
	}
	createdUser, found, err := s.GetUser(ctx, user.TelegramID)
	if err != nil || !found {
		return err
	}
	return s.saveUserMessengerAccount(ctx, domain.UserMessengerAccount{
		UserID:         createdUser.ID,
		MessengerKind:  domain.MessengerKindTelegram,
		ExternalUserID: strconv.FormatInt(user.TelegramID, 10),
		Username:       user.TelegramUsername,
		LinkedAt:       now,
		UpdatedAt:      now,
	})
}

// GetUserByID fetches user by internal user ID.
func (s *Store) GetUserByID(ctx context.Context, userID int64) (domain.User, bool, error) {
	var u domain.User
	err := s.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(telegram_id, 0), telegram_username, full_name, phone, email, updated_at
		FROM users
		WHERE id = $1
	`, userID).Scan(&u.ID, &u.TelegramID, &u.TelegramUsername, &u.FullName, &u.Phone, &u.Email, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return domain.User{}, false, nil
	}
	if err != nil {
		return domain.User{}, false, err
	}
	return u, true, nil
}

// GetUser fetches user by Telegram ID.
func (s *Store) GetUser(ctx context.Context, telegramID int64) (domain.User, bool, error) {
	var u domain.User
	err := s.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(telegram_id, 0), telegram_username, full_name, phone, email, updated_at
		FROM users
		WHERE telegram_id = $1
	`, telegramID).Scan(&u.ID, &u.TelegramID, &u.TelegramUsername, &u.FullName, &u.Phone, &u.Email, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return domain.User{}, false, nil
	}
	if err != nil {
		return domain.User{}, false, err
	}
	return u, true, nil
}

// GetUserByMessenger resolves an existing user by external messenger identity without creating one.
func (s *Store) GetUserByMessenger(ctx context.Context, kind domain.MessengerKind, externalUserID string) (domain.User, bool, error) {
	var u domain.User
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id, COALESCE(u.telegram_id, 0), u.telegram_username, u.full_name, u.phone, u.email, u.updated_at
		FROM user_messenger_accounts a
		JOIN users u ON u.id = a.user_id
		WHERE a.messenger_kind = $1 AND a.external_user_id = $2
	`, string(kind), externalUserID).Scan(&u.ID, &u.TelegramID, &u.TelegramUsername, &u.FullName, &u.Phone, &u.Email, &u.UpdatedAt)
	if err == nil {
		return u, true, nil
	}
	if err != sql.ErrNoRows {
		return domain.User{}, false, err
	}
	if kind != domain.MessengerKindTelegram {
		return domain.User{}, false, nil
	}

	telegramID, parseErr := strconv.ParseInt(externalUserID, 10, 64)
	if parseErr != nil || telegramID <= 0 {
		return domain.User{}, false, nil
	}
	return s.GetUser(ctx, telegramID)
}

// GetOrCreateUserByMessenger resolves a user by external messenger identity and creates one if absent.
func (s *Store) GetOrCreateUserByMessenger(ctx context.Context, kind domain.MessengerKind, externalUserID, username string) (domain.User, bool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return domain.User{}, false, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var existing domain.User
	err = tx.QueryRowContext(ctx, `
		SELECT u.id, COALESCE(u.telegram_id, 0), u.telegram_username, u.full_name, u.phone, u.email, u.updated_at
		FROM user_messenger_accounts a
		JOIN users u ON u.id = a.user_id
		WHERE a.messenger_kind = $1 AND a.external_user_id = $2
	`, string(kind), externalUserID).Scan(
		&existing.ID,
		&existing.TelegramID,
		&existing.TelegramUsername,
		&existing.FullName,
		&existing.Phone,
		&existing.Email,
		&existing.UpdatedAt,
	)
	if err == nil {
		if strings.TrimSpace(username) != "" {
			if _, err := tx.ExecContext(ctx, `
				UPDATE user_messenger_accounts
				SET username = $3, updated_at = $4
				WHERE messenger_kind = $1 AND external_user_id = $2
			`, string(kind), externalUserID, username, time.Now().UTC()); err != nil {
				return domain.User{}, false, err
			}
			if kind == domain.MessengerKindTelegram && existing.TelegramUsername != username {
				if _, err := tx.ExecContext(ctx, `
					UPDATE users
					SET telegram_username = $2, updated_at = $3
					WHERE id = $1
				`, existing.ID, username, time.Now().UTC()); err != nil {
					return domain.User{}, false, err
				}
				existing.TelegramUsername = username
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

	var telegramID sql.NullInt64
	var telegramUsername string
	if kind == domain.MessengerKindTelegram {
		parsedID, parseErr := strconv.ParseInt(externalUserID, 10, 64)
		if parseErr != nil || parsedID <= 0 {
			return domain.User{}, false, fmt.Errorf("invalid telegram external user id")
		}
		telegramID = sql.NullInt64{Int64: parsedID, Valid: true}
		telegramUsername = username

		err = tx.QueryRowContext(ctx, `
			SELECT id, COALESCE(telegram_id, 0), telegram_username, full_name, phone, email, updated_at
			FROM users
			WHERE telegram_id = $1
		`, parsedID).Scan(
			&existing.ID,
			&existing.TelegramID,
			&existing.TelegramUsername,
			&existing.FullName,
			&existing.Phone,
			&existing.Email,
			&existing.UpdatedAt,
		)
		if err == nil {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO user_messenger_accounts (
					user_id, messenger_kind, external_user_id, username, linked_at, updated_at
				) VALUES ($1,$2,$3,$4,$5,$6)
				ON CONFLICT (messenger_kind, external_user_id)
				DO UPDATE SET
					user_id = EXCLUDED.user_id,
					username = EXCLUDED.username,
					updated_at = EXCLUDED.updated_at
			`, existing.ID, string(kind), externalUserID, username, time.Now().UTC(), time.Now().UTC()); err != nil {
				return domain.User{}, false, err
			}
			if err := tx.Commit(); err != nil {
				return domain.User{}, false, err
			}
			return existing, false, nil
		}
		if err != sql.ErrNoRows {
			return domain.User{}, false, err
		}
	}

	var created domain.User
	err = tx.QueryRowContext(ctx, `
		INSERT INTO users (telegram_id, telegram_username, full_name, phone, email, updated_at)
		VALUES ($1,$2,'','','',$3)
		RETURNING id, COALESCE(telegram_id, 0), telegram_username, full_name, phone, email, updated_at
	`, telegramID, telegramUsername, time.Now().UTC()).Scan(
		&created.ID,
		&created.TelegramID,
		&created.TelegramUsername,
		&created.FullName,
		&created.Phone,
		&created.Email,
		&created.UpdatedAt,
	)
	if err != nil {
		return domain.User{}, false, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_messenger_accounts (
			user_id, messenger_kind, external_user_id, username, linked_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6)
	`, created.ID, string(kind), externalUserID, username, time.Now().UTC(), time.Now().UTC()); err != nil {
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
		SELECT user_id, messenger_kind, external_user_id, username, linked_at, updated_at
		FROM user_messenger_accounts
		WHERE user_id = $1
		ORDER BY messenger_kind ASC, external_user_id ASC
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
			&item.ExternalUserID,
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
			u.id,
			COALESCE(u.telegram_id, 0) AS telegram_id,
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

func nullableTelegramID(telegramID int64) sql.NullInt64 {
	if telegramID <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: telegramID, Valid: true}
}

func (s *Store) resolveUserIdentity(ctx context.Context, userID, telegramID int64) (int64, int64, error) {
	if userID > 0 {
		user, found, err := s.GetUserByID(ctx, userID)
		if err != nil {
			return 0, 0, err
		}
		if found {
			if telegramID <= 0 {
				telegramID = user.TelegramID
			}
			return user.ID, telegramID, nil
		}
	}
	if telegramID > 0 {
		user, found, err := s.GetUser(ctx, telegramID)
		if err != nil {
			return 0, 0, err
		}
		if found {
			if userID <= 0 {
				userID = user.ID
			}
			return userID, telegramID, nil
		}
	}
	return userID, telegramID, nil
}

func (s *Store) saveUserMessengerAccount(ctx context.Context, account domain.UserMessengerAccount) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO user_messenger_accounts (
			user_id, messenger_kind, external_user_id, username, linked_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (messenger_kind, external_user_id)
		DO UPDATE SET
			user_id = EXCLUDED.user_id,
			username = EXCLUDED.username,
			updated_at = EXCLUDED.updated_at
	`, account.UserID, string(account.MessengerKind), account.ExternalUserID, account.Username, account.LinkedAt, account.UpdatedAt)
	return err
}
