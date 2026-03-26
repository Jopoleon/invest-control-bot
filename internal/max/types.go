package max

import "encoding/json"

// GetUpdatesRequest mirrors MAX long-polling query parameters.
type GetUpdatesRequest struct {
	Limit      int
	TimeoutSec int
	Marker     *int64
	Types      []string
}

// CreateWebhookSubscriptionRequest configures MAX webhook delivery.
type CreateWebhookSubscriptionRequest struct {
	URL         string   `json:"url"`
	UpdateTypes []string `json:"update_types,omitempty"`
	Secret      string   `json:"secret,omitempty"`
}

// UpdatesPage is one page returned by GET /updates.
type UpdatesPage struct {
	Updates []Update `json:"updates"`
	Marker  *int64   `json:"marker"`
}

// Update keeps the transport envelope intentionally small for the first
// adapter step. Message-specific payloads can be expanded as we learn the
// production payloads delivered by MAX.
type Update struct {
	UpdateType   string          `json:"update_type"`
	Timestamp    int64           `json:"timestamp,omitempty"`
	User         *User           `json:"user,omitempty"`
	ChatID       int64           `json:"chat_id,omitempty"`
	StartPayload string          `json:"start_payload,omitempty"`
	Payload      string          `json:"payload,omitempty"`
	Message      *Message        `json:"message,omitempty"`
	Callback     *Callback       `json:"callback,omitempty"`
	Raw          json.RawMessage `json:"-"`
}

func (u *Update) UnmarshalJSON(data []byte) error {
	type alias Update
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*u = Update(decoded)
	u.Raw = append(u.Raw[:0], data...)
	return nil
}

// Message is the subset of MAX message fields we need for polling and replies.
type Message struct {
	MessageID int64           `json:"message_id,omitempty"`
	Sender    *User           `json:"sender,omitempty"`
	Recipient *Recipient      `json:"recipient,omitempty"`
	Timestamp int64           `json:"timestamp,omitempty"`
	Text      string          `json:"text,omitempty"`
	Body      *MessageBody    `json:"body,omitempty"`
	Raw       json.RawMessage `json:"-"`
}

func (m *Message) UnmarshalJSON(data []byte) error {
	type alias Message
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = Message(decoded)
	m.Raw = append(m.Raw[:0], data...)
	return nil
}

type Recipient struct {
	ChatID   int64  `json:"chat_id,omitempty"`
	UserID   int64  `json:"user_id,omitempty"`
	ChatType string `json:"chat_type,omitempty"`
	Type     string `json:"type,omitempty"`
	Name     string `json:"name,omitempty"`
}

type MessageBody struct {
	MID  string `json:"mid,omitempty"`
	Text string `json:"text,omitempty"`
}

// User is the subset of MAX user payload we need for identity linking.
type User struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username,omitempty"`
	Name     string `json:"name,omitempty"`
}

// Callback is the minimal callback payload required by the bot core.
type Callback struct {
	CallbackID string   `json:"callback_id"`
	User       *User    `json:"user,omitempty"`
	Message    *Message `json:"message,omitempty"`
	Payload    string   `json:"payload,omitempty"`
	Data       string   `json:"data,omitempty"`
}

// SendMessageRequest is a minimal request for POST /messages.
type SendMessageRequest struct {
	UserID             int64
	ChatID             int64
	Text               string
	DisableLinkPreview bool
	Notify             *bool
	Format             string
	Attachments        []Attachment `json:"attachments,omitempty"`
}

type newMessageBody struct {
	Text        string       `json:"text,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Notify      *bool        `json:"notify,omitempty"`
	Format      string       `json:"format,omitempty"`
}

type sendMessageResponse struct {
	Message Message `json:"message"`
}

type AnswerCallbackRequest struct {
	Message      *newMessageBody `json:"message,omitempty"`
	Notification string          `json:"notification,omitempty"`
}

// Attachment is enough for inline keyboard buttons in the first MAX MVP.
type Attachment struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type InlineKeyboardPayload struct {
	Buttons [][]InlineKeyboardButton `json:"buttons"`
}

type InlineKeyboardButton struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	URL     string `json:"url,omitempty"`
	Payload string `json:"payload,omitempty"`
}
