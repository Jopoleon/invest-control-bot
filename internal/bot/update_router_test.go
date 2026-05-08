package bot

import (
	"context"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
	"github.com/go-telegram/bot/models"
)

func TestHandleUpdate_SavesTelegramChatFromMyChatMember(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, nil, nil, false, "", "")
	seenAt := time.Date(2026, 4, 30, 9, 10, 0, 0, time.UTC)

	h.HandleUpdate(ctx, &models.Update{
		MyChatMember: &models.ChatMemberUpdated{
			Chat: models.Chat{
				ID:       -1001234567890,
				Type:     "supergroup",
				Title:    "Paid Test Chat",
				Username: "paid_test_chat",
			},
			Date: int(seenAt.Unix()),
			NewChatMember: models.ChatMember{
				Type: models.ChatMemberTypeAdministrator,
				Administrator: &models.ChatMemberAdministrator{
					CanInviteUsers:     true,
					CanRestrictMembers: true,
					CanManageChat:      true,
				},
			},
		},
	})

	chat, found, err := st.GetTelegramChat(ctx, "-1001234567890")
	if err != nil {
		t.Fatalf("GetTelegramChat err=%v", err)
	}
	if !found {
		t.Fatalf("telegram chat was not saved")
	}
	if chat.Title != "Paid Test Chat" || chat.Type != "supergroup" || chat.BotStatus != "administrator" {
		t.Fatalf("unexpected chat: %+v", chat)
	}
	if !chat.CanInviteUsers || !chat.CanRestrictMembers || !chat.CanManageChat {
		t.Fatalf("admin rights were not captured: %+v", chat)
	}
	if !chat.LastSeenAt.Equal(seenAt) {
		t.Fatalf("last_seen_at=%s want=%s", chat.LastSeenAt, seenAt)
	}
}
