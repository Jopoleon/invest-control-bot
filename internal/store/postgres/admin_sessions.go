package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (s *Store) CreateAdminSession(ctx context.Context, session domain.AdminSession) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO admin_sessions (
			session_token_hash, subject, created_at, expires_at, last_seen_at,
			revoked_at, ip, user_agent, rotated_at, replaced_by_hash
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`,
		session.TokenHash,
		session.Subject,
		session.CreatedAt,
		session.ExpiresAt,
		session.LastSeenAt,
		session.RevokedAt,
		session.IP,
		session.UserAgent,
		session.RotatedAt,
		session.ReplacedByHash,
	)
	return err
}

// ListAdminSessions returns recent admin sessions for operational UI.
func (s *Store) ListAdminSessions(ctx context.Context, limit int) ([]domain.AdminSession, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_token_hash, subject, created_at, expires_at, last_seen_at,
		       revoked_at, ip, user_agent, rotated_at, replaced_by_hash
		FROM admin_sessions
		ORDER BY created_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]domain.AdminSession, 0, limit)
	for rows.Next() {
		item, err := scanAdminSession(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetAdminSessionByTokenHash fetches admin session by token hash.
func (s *Store) GetAdminSessionByTokenHash(ctx context.Context, tokenHash string) (domain.AdminSession, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, session_token_hash, subject, created_at, expires_at, last_seen_at,
		       revoked_at, ip, user_agent, rotated_at, replaced_by_hash
		FROM admin_sessions
		WHERE session_token_hash = $1
		LIMIT 1
	`, tokenHash)
	session, err := scanAdminSession(row)
	if err == sql.ErrNoRows {
		return domain.AdminSession{}, false, nil
	}
	if err != nil {
		return domain.AdminSession{}, false, err
	}
	return session, true, nil
}

// TouchAdminSession updates last access timestamp.
func (s *Store) TouchAdminSession(ctx context.Context, sessionID int64, lastSeenAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE admin_sessions
		SET last_seen_at = $2
		WHERE id = $1
	`, sessionID, lastSeenAt)
	return err
}

// RotateAdminSession replaces token hash for active session.
func (s *Store) RotateAdminSession(ctx context.Context, sessionID int64, newTokenHash string, rotatedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE admin_sessions
		SET replaced_by_hash = $2,
		    session_token_hash = $2,
		    rotated_at = $3,
		    last_seen_at = $3
		WHERE id = $1
	`, sessionID, newTokenHash, rotatedAt)
	return err
}

// RevokeAdminSession marks session revoked.
func (s *Store) RevokeAdminSession(ctx context.Context, sessionID int64, revokedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE admin_sessions
		SET revoked_at = $2
		WHERE id = $1
	`, sessionID, revokedAt)
	return err
}

// CleanupAdminSessions deletes revoked and absolute-expired admin sessions.
func (s *Store) CleanupAdminSessions(ctx context.Context, expiredBefore time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM admin_sessions
		WHERE revoked_at IS NOT NULL OR expires_at < $1
	`, expiredBefore)
	return err
}
