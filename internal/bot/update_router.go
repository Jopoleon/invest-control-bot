package bot

import (
	"context"
	"strconv"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/go-telegram/bot/models"
)

// HandleUpdate is the Telegram transport boundary: it maps Telegram DTOs to
// internal messenger-neutral events and keeps the rest of the bot package free
// from direct Telegram API types.
func (h *Handler) HandleUpdate(ctx context.Context, update *models.Update) {
	if update == nil {
		return
	}
	if update.MyChatMember != nil {
		h.handleTelegramBotChatMemberUpdate(ctx, update.MyChatMember)
		return
	}
	if update.CallbackQuery != nil {
		cb := update.CallbackQuery
		if cb.Message.Message == nil {
			return
		}
		h.handleCallback(ctx, messenger.IncomingAction{
			Ref:       actionRef(cb.ID),
			User:      userIdentity(cb.From.ID, cb.From.Username),
			ChatID:    cb.Message.Message.Chat.ID,
			MessageID: cb.Message.Message.ID,
			Data:      cb.Data,
		})
		return
	}
	if update.Message != nil {
		msg := update.Message
		h.handleMessage(ctx, messenger.IncomingMessage{
			User:   userIdentity(msg.From.ID, msg.From.Username),
			ChatID: msg.Chat.ID,
			Text:   msg.Text,
		})
	}
}

func (h *Handler) handleTelegramBotChatMemberUpdate(ctx context.Context, update *models.ChatMemberUpdated) {
	if update == nil || update.Chat.ID == 0 {
		return
	}
	status, canInviteUsers, canRestrictMembers, canManageChat := telegramBotChatRights(update.NewChatMember)
	now := time.Now().UTC()
	seenAt := now
	if update.Date > 0 {
		seenAt = time.Unix(int64(update.Date), 0).UTC()
	}

	// This is the one reliable Telegram-originated signal we can use to build
	// the admin chat catalog. Telegram does not let a bot enumerate all chats
	// where it is an admin after the fact, so operators can re-add/change bot
	// rights in existing chats once and this handler captures the chat_id.
	_ = h.store.UpsertTelegramChat(ctx, domain.TelegramChat{
		ChatID:             strconv.FormatInt(update.Chat.ID, 10),
		Title:              update.Chat.Title,
		Username:           update.Chat.Username,
		Type:               string(update.Chat.Type),
		BotStatus:          status,
		CanInviteUsers:     canInviteUsers,
		CanRestrictMembers: canRestrictMembers,
		CanManageChat:      canManageChat,
		LastSeenAt:         seenAt,
		UpdatedAt:          now,
	})
}

func telegramBotChatRights(member models.ChatMember) (status string, canInviteUsers, canRestrictMembers, canManageChat bool) {
	status = string(member.Type)
	if member.Administrator != nil {
		return status,
			member.Administrator.CanInviteUsers,
			member.Administrator.CanRestrictMembers,
			member.Administrator.CanManageChat
	}
	if member.Owner != nil {
		return status, true, true, true
	}
	return status, false, false, false
}
