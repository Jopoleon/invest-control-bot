package max

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/bot"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// Adapter maps MAX updates into the shared bot core.
type Adapter struct {
	handler *bot.Handler
}

func NewAdapter(handler *bot.Handler) *Adapter {
	return &Adapter{handler: handler}
}

// Dispatch routes one MAX update into the transport-neutral bot handler.
func (a *Adapter) Dispatch(ctx context.Context, update Update) {
	if a == nil || a.handler == nil {
		return
	}

	switch update.UpdateType {
	case "message_created":
		msg, ok := mapIncomingMessage(update)
		if !ok {
			slog.Debug("max update skipped: failed to map message_created", "update", stringifyUpdate(update))
			return
		}
		slog.Debug("max update mapped to incoming message",
			"update_type", update.UpdateType,
			"user_id", msg.User.ID,
			"username", msg.User.Username,
			"chat_id", msg.ChatID,
			"text", msg.Text,
			"sender_user_id", userIDOrZero(messageSender(update.Message)),
			"recipient_user_id", recipientUserID(update.Message.Recipient),
			"recipient_chat_id", recipientChatID(update.Message.Recipient),
			"message_mid", messageMID(update.Message),
		)
		a.handler.HandleIncomingMessage(ctx, msg)
	case "message_callback":
		action, ok := mapIncomingAction(update)
		if !ok {
			slog.Debug("max update skipped: failed to map message_callback", "update", stringifyUpdate(update))
			return
		}
		callbackMsg := callbackMessage(update)
		slog.Debug("max update mapped to incoming action",
			"update_type", update.UpdateType,
			"user_id", action.User.ID,
			"username", action.User.Username,
			"chat_id", action.ChatID,
			"message_id", action.MessageID,
			"action_id", action.Ref.ID,
			"data", action.Data,
			"sender_user_id", userIDOrZero(messageSender(callbackMsg)),
			"recipient_user_id", recipientUserID(callbackRecipient(callbackMsg)),
			"recipient_chat_id", recipientChatID(callbackRecipient(callbackMsg)),
			"message_mid", messageMID(callbackMsg),
		)
		a.handler.HandleIncomingAction(ctx, action)
	case "bot_started":
		msg, ok := mapBotStarted(update)
		if !ok {
			slog.Debug("max update skipped: failed to map bot_started", "update", stringifyUpdate(update))
			return
		}
		slog.Debug("max update mapped to bot_started message",
			"update_type", update.UpdateType,
			"user_id", msg.User.ID,
			"username", msg.User.Username,
			"chat_id", msg.ChatID,
			"text", msg.Text,
			"start_payload", update.StartPayload,
			"payload", update.Payload,
		)
		a.handler.HandleIncomingMessage(ctx, msg)
	default:
		slog.Debug("max update skipped: unsupported type", "update_type", update.UpdateType)
	}
}

func mapIncomingMessage(update Update) (messenger.IncomingMessage, bool) {
	if update.Message == nil {
		return messenger.IncomingMessage{}, false
	}
	user := parseUserIdentity(firstNonNilUser(update.Message.Sender, update.User))
	if user.ID <= 0 {
		return messenger.IncomingMessage{}, false
	}
	chatID := incomingChatID(update.Message.Recipient, user.ID, update.ChatID)
	text := strings.TrimSpace(parseMessageText(update.Message))
	if chatID <= 0 || text == "" {
		return messenger.IncomingMessage{}, false
	}
	return messenger.IncomingMessage{
		User:   user,
		ChatID: chatID,
		Text:   text,
	}, true
}

func mapIncomingAction(update Update) (messenger.IncomingAction, bool) {
	if update.Callback == nil {
		return messenger.IncomingAction{}, false
	}
	msg := callbackMessage(update)
	user := parseUserIdentity(firstNonNilUser(update.Callback.User, messageSender(msg), update.User))
	if user.ID <= 0 {
		return messenger.IncomingAction{}, false
	}
	data := strings.TrimSpace(parseCallbackData(update.Callback))
	if data == "" {
		return messenger.IncomingAction{}, false
	}
	chatID := user.ID
	messageID := 0
	if msg != nil {
		chatID = incomingChatID(msg.Recipient, user.ID, update.ChatID)
		messageID = parseMessageID(msg)
	} else if update.ChatID > 0 {
		chatID = update.ChatID
	}

	return messenger.IncomingAction{
		Ref: messenger.ActionRef{
			Kind: messenger.KindMAX,
			ID:   update.Callback.CallbackID,
		},
		User:      user,
		ChatID:    chatID,
		MessageID: messageID,
		Data:      data,
	}, true
}

func mapBotStarted(update Update) (messenger.IncomingMessage, bool) {
	user := parseUserIdentity(firstNonNilUser(update.User, messageSender(update.Message), callbackUser(update.Callback)))
	if user.ID <= 0 {
		return messenger.IncomingMessage{}, false
	}
	chatID := firstNonZeroInt64(update.ChatID, user.ID)
	if update.Message != nil {
		chatID = incomingChatID(update.Message.Recipient, user.ID, chatID)
	}
	if chatID <= 0 {
		return messenger.IncomingMessage{}, false
	}
	text := "/start"
	payload := strings.TrimSpace(firstNonEmpty(
		update.StartPayload,
		update.Payload,
		extractStartPayload(parseMessageText(update.Message)),
	))
	if payload != "" {
		text += " " + payload
	}
	return messenger.IncomingMessage{
		User:   user,
		ChatID: chatID,
		Text:   text,
	}, true
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstNonNilUser(values ...*User) *User {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func messageSender(msg *Message) *User {
	if msg == nil {
		return nil
	}
	return msg.Sender
}

func callbackMessage(update Update) *Message {
	if update.Callback != nil && update.Callback.Message != nil {
		return update.Callback.Message
	}
	return update.Message
}

func callbackUser(callback *Callback) *User {
	if callback == nil {
		return nil
	}
	return callback.User
}

func callbackRecipient(msg *Message) *Recipient {
	if msg == nil {
		return nil
	}
	return msg.Recipient
}

func recipientChatID(recipient *Recipient) int64 {
	if recipient == nil {
		return 0
	}
	return firstNonZeroInt64(recipient.ChatID, recipient.UserID)
}

func incomingChatID(recipient *Recipient, senderUserID, fallback int64) int64 {
	if recipient != nil && strings.EqualFold(strings.TrimSpace(recipient.ChatType), "dialog") && senderUserID > 0 {
		return senderUserID
	}
	if recipient != nil && recipient.ChatID > 0 {
		return recipient.ChatID
	}
	if senderUserID > 0 {
		return senderUserID
	}
	return firstNonZeroInt64(recipientChatID(recipient), fallback)
}

func stringifyUpdate(update Update) string {
	if len(update.Raw) > 0 {
		return string(update.Raw)
	}
	raw, err := json.Marshal(update)
	if err != nil {
		return ""
	}
	return string(raw)
}

func userIDOrZero(user *User) int64 {
	if user == nil {
		return 0
	}
	return user.UserID
}

func recipientUserID(recipient *Recipient) int64 {
	if recipient == nil {
		return 0
	}
	return recipient.UserID
}

func messageMID(msg *Message) string {
	if msg == nil || msg.Body == nil {
		return ""
	}
	return msg.Body.MID
}

func extractStartPayload(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if text == "/start" {
		return ""
	}
	if strings.HasPrefix(text, "/start ") {
		return strings.TrimSpace(strings.TrimPrefix(text, "/start "))
	}
	return ""
}
