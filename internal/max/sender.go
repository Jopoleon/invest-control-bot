package max

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// Sender adapts MAX HTTP API to the transport-neutral messenger.Sender contract.
type Sender struct {
	client *Client
}

func NewSender(client *Client) *Sender {
	return &Sender{client: client}
}

func (s *Sender) Send(ctx context.Context, user messenger.UserRef, msg messenger.OutgoingMessage) error {
	attachments := toMAXAttachments(msg.Buttons)
	req := SendMessageRequest{
		Text:        msg.Text,
		Attachments: attachments,
	}
	switch {
	case user.UserID > 0:
		req.UserID = user.UserID
	case user.ChatID > 0:
		req.ChatID = user.ChatID
	}
	slog.Debug("max sender send request",
		"user_id", req.UserID,
		"chat_id", req.ChatID,
		"text", req.Text,
		"attachments", len(req.Attachments),
		"button_rows", len(msg.Buttons),
	)
	_, err := s.client.SendMessage(ctx, req)
	return err
}

func (s *Sender) Edit(ctx context.Context, ref messenger.MessageRef, msg messenger.OutgoingMessage) error {
	req := SendMessageRequest{
		Text:        msg.Text,
		Attachments: toMAXAttachments(msg.Buttons),
	}
	slog.Debug("max sender edit request",
		"chat_id", ref.ChatID,
		"message_id", ref.MessageID,
		"text", req.Text,
		"attachments", len(req.Attachments),
		"button_rows", len(msg.Buttons),
	)
	return s.client.EditMessage(ctx, int64(ref.MessageID), req)
}

func (s *Sender) AnswerAction(ctx context.Context, ref messenger.ActionRef, text string) error {
	if ref.ID == "" || strings.TrimSpace(text) == "" {
		slog.Debug("max sender answer action skipped", "action_id", ref.ID, "text", text)
		return nil
	}
	slog.Debug("max sender answer action request", "action_id", ref.ID, "text", text)
	return s.client.AnswerCallback(ctx, ref.ID, AnswerCallbackRequest{Notification: text})
}

func toMAXAttachments(rows [][]messenger.ActionButton) []Attachment {
	if len(rows) == 0 {
		return nil
	}
	payloadRows := make([][]map[string]string, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		payloadRow := make([]map[string]string, 0, len(row))
		for _, button := range row {
			switch {
			case button.URL != "":
				payloadRow = append(payloadRow, map[string]string{
					"type": "link",
					"text": button.Text,
					"url":  button.URL,
				})
			case button.Action != "":
				payloadRow = append(payloadRow, map[string]string{
					"type":    "callback",
					"text":    button.Text,
					"payload": button.Action,
				})
			}
		}
		if len(payloadRow) > 0 {
			payloadRows = append(payloadRows, payloadRow)
		}
	}
	if len(payloadRows) == 0 {
		return nil
	}
	return []Attachment{{
		Type: "inline_keyboard",
		Payload: map[string]any{
			"buttons": payloadRows,
		},
	}}
}

func parseMessageText(msg *Message) string {
	if msg == nil {
		return ""
	}
	if text := strings.TrimSpace(msg.Text); text != "" {
		return text
	}
	if msg.Body != nil {
		return strings.TrimSpace(msg.Body.Text)
	}
	return ""
}

func parseMessageID(msg *Message) int {
	if msg == nil {
		return 0
	}
	if msg.Body != nil && strings.TrimSpace(msg.Body.MID) != "" {
		value, err := strconv.ParseInt(strings.TrimSpace(msg.Body.MID), 10, 64)
		if err == nil && value > 0 && value <= int64(^uint(0)>>1) {
			return int(value)
		}
	}
	if msg.MessageID <= 0 || msg.MessageID > int64(^uint(0)>>1) {
		return 0
	}
	return int(msg.MessageID)
}

func parseUserIdentity(updateUser *User) messenger.UserIdentity {
	if updateUser == nil {
		return messenger.UserIdentity{Kind: messenger.KindMAX}
	}
	return messenger.UserIdentity{
		Kind:     messenger.KindMAX,
		ID:       updateUser.UserID,
		Username: firstNonEmpty(updateUser.Username, updateUser.Name),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func parseCallbackData(callback *Callback) string {
	if callback == nil {
		return ""
	}
	if callback.Payload != "" {
		return callback.Payload
	}
	return callback.Data
}
