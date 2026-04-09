package memory

import (
	"context"
	"sort"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (s *Store) SaveTelegramInviteLink(_ context.Context, link domain.TelegramInviteLink) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if link.CreatedAt.IsZero() {
		link.CreatedAt = time.Now().UTC()
	}
	link.UserID, _ = s.resolveUserIdentityLocked(link.UserID, 0)
	link.ID = s.nextTelegramInviteID
	s.nextTelegramInviteID++
	s.telegramInviteLinks[link.ID] = link
	return nil
}

func (s *Store) ListActiveTelegramInviteLinks(_ context.Context, userID int64, chatRef string, now time.Time) ([]domain.TelegramInviteLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if now.IsZero() {
		now = time.Now().UTC()
	}
	out := make([]domain.TelegramInviteLink, 0, len(s.telegramInviteLinks))
	for _, link := range s.telegramInviteLinks {
		if link.UserID != userID || link.ChatRef != chatRef {
			continue
		}
		if link.RevokedAt != nil {
			continue
		}
		if link.ExpiresAt != nil && !link.ExpiresAt.After(now) {
			continue
		}
		out = append(out, link)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) ListRevocableTelegramInviteLinks(_ context.Context, userID int64, chatRef string) ([]domain.TelegramInviteLink, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]domain.TelegramInviteLink, 0, len(s.telegramInviteLinks))
	for _, link := range s.telegramInviteLinks {
		if link.UserID != userID || link.ChatRef != chatRef {
			continue
		}
		if link.RevokedAt != nil {
			continue
		}
		out = append(out, link)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) MarkTelegramInviteLinkRevoked(_ context.Context, inviteLinkID int64, revokedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	link, ok := s.telegramInviteLinks[inviteLinkID]
	if !ok {
		return nil
	}
	if revokedAt.IsZero() {
		revokedAt = time.Now().UTC()
	}
	link.RevokedAt = &revokedAt
	s.telegramInviteLinks[inviteLinkID] = link
	return nil
}
