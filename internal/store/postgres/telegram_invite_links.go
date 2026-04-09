package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (s *Store) SaveTelegramInviteLink(ctx context.Context, link domain.TelegramInviteLink) error {
	now := time.Now().UTC()
	if link.CreatedAt.IsZero() {
		link.CreatedAt = now
	}
	resolvedUserID, _, err := s.resolveUserIdentity(ctx, link.UserID, 0)
	if err != nil {
		return err
	}
	link.UserID = resolvedUserID
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO telegram_invite_links (
			user_id, connector_id, subscription_id, chat_ref, invite_link, expires_at, revoked_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`,
		link.UserID,
		link.ConnectorID,
		link.SubscriptionID,
		link.ChatRef,
		link.InviteLink,
		link.ExpiresAt,
		link.RevokedAt,
		link.CreatedAt,
	)
	return err
}

func (s *Store) ListActiveTelegramInviteLinks(ctx context.Context, userID int64, chatRef string, now time.Time) ([]domain.TelegramInviteLink, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, connector_id, subscription_id, chat_ref, invite_link, expires_at, revoked_at, created_at
		FROM telegram_invite_links
		WHERE user_id = $1
		  AND chat_ref = $2
		  AND revoked_at IS NULL
		  AND (expires_at IS NULL OR expires_at > $3)
		ORDER BY created_at DESC, id DESC
	`, userID, chatRef, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	links := make([]domain.TelegramInviteLink, 0, 8)
	for rows.Next() {
		item, err := scanTelegramInviteLink(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, item)
	}
	return links, rows.Err()
}

func (s *Store) ListRevocableTelegramInviteLinks(ctx context.Context, userID int64, chatRef string) ([]domain.TelegramInviteLink, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, connector_id, subscription_id, chat_ref, invite_link, expires_at, revoked_at, created_at
		FROM telegram_invite_links
		WHERE user_id = $1
		  AND chat_ref = $2
		  AND revoked_at IS NULL
		ORDER BY created_at DESC, id DESC
	`, userID, chatRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	links := make([]domain.TelegramInviteLink, 0, 8)
	for rows.Next() {
		item, err := scanTelegramInviteLink(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, item)
	}
	return links, rows.Err()
}

func (s *Store) MarkTelegramInviteLinkRevoked(ctx context.Context, inviteLinkID int64, revokedAt time.Time) error {
	if revokedAt.IsZero() {
		revokedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE telegram_invite_links
		SET revoked_at = $2
		WHERE id = $1
		  AND revoked_at IS NULL
	`, inviteLinkID, revokedAt)
	return err
}

func (s *Store) getTelegramInviteLinkByURL(ctx context.Context, inviteLink string) (domain.TelegramInviteLink, bool, error) {
	item, err := scanTelegramInviteLink(s.db.QueryRowContext(ctx, `
		SELECT id, user_id, connector_id, subscription_id, chat_ref, invite_link, expires_at, revoked_at, created_at
		FROM telegram_invite_links
		WHERE invite_link = $1
	`, inviteLink))
	if err == sql.ErrNoRows {
		return domain.TelegramInviteLink{}, false, nil
	}
	if err != nil {
		return domain.TelegramInviteLink{}, false, err
	}
	return item, true, nil
}
