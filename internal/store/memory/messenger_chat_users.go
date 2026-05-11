package memory

import (
	"context"
	"sort"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (s *Store) UpsertMessengerChatUser(_ context.Context, user domain.MessengerChatUser) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.messengerChatUsers[messengerChatUserKey(user.MessengerKind, user.ChatRef, user.MessengerUserID)] = user
	return nil
}

func (s *Store) GetMessengerChatUser(_ context.Context, kind domain.MessengerKind, chatRef, messengerUserID string) (domain.MessengerChatUser, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	item, ok := s.messengerChatUsers[messengerChatUserKey(kind, chatRef, messengerUserID)]
	return item, ok, nil
}

func (s *Store) ListMessengerChatUsersByUser(_ context.Context, userID int64) ([]domain.MessengerChatUser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]domain.MessengerChatUser, 0, len(s.messengerChatUsers))
	for _, item := range s.messengerChatUsers {
		if item.UserID == userID {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ChatRef < items[j].ChatRef
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

func (s *Store) SaveMessengerChatUserCheck(_ context.Context, check domain.MessengerChatUserCheck) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	check.ID = s.nextMessengerCheckID
	s.nextMessengerCheckID++
	check.RawJSON = append([]byte(nil), check.RawJSON...)
	s.messengerChatChecks = append(s.messengerChatChecks, check)
	return nil
}

func (s *Store) ListMessengerChatUserChecks(_ context.Context, kind domain.MessengerKind, chatRef, messengerUserID string, limit int) ([]domain.MessengerChatUserCheck, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}
	items := make([]domain.MessengerChatUserCheck, 0, limit)
	for i := len(s.messengerChatChecks) - 1; i >= 0 && len(items) < limit; i-- {
		item := s.messengerChatChecks[i]
		if item.MessengerKind == kind && item.ChatRef == chatRef && item.MessengerUserID == messengerUserID {
			item.RawJSON = append([]byte(nil), item.RawJSON...)
			items = append(items, item)
		}
	}
	return items, nil
}

func messengerChatUserKey(kind domain.MessengerKind, chatRef, messengerUserID string) string {
	return strings.Join([]string{string(kind), strings.TrimSpace(chatRef), strings.TrimSpace(messengerUserID)}, "\x00")
}
