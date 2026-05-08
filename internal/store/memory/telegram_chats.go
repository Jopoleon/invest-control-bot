package memory

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (s *Store) UpsertTelegramChat(_ context.Context, chat domain.TelegramChat) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if chat.LastSeenAt.IsZero() {
		chat.LastSeenAt = now
	}
	if chat.UpdatedAt.IsZero() {
		chat.UpdatedAt = now
	}
	s.telegramChats[chat.ChatID] = chat
	return nil
}

func (s *Store) ListTelegramChats(_ context.Context) ([]domain.TelegramChat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]domain.TelegramChat, 0, len(s.telegramChats))
	for _, chat := range s.telegramChats {
		items = append(items, chat)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].BotStatus == "administrator" && items[j].BotStatus != "administrator" {
			return true
		}
		if items[i].BotStatus != "administrator" && items[j].BotStatus == "administrator" {
			return false
		}
		left := strings.ToLower(items[i].Title)
		right := strings.ToLower(items[j].Title)
		if left == right {
			return items[i].ChatID < items[j].ChatID
		}
		return left < right
	})
	return items, nil
}

func (s *Store) GetTelegramChat(_ context.Context, chatID string) (domain.TelegramChat, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	chat, ok := s.telegramChats[chatID]
	return chat, ok, nil
}
