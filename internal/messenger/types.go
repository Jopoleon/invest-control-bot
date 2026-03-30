package messenger

import "context"

// Kind identifies a transport implementation.
type Kind string

const (
	KindTelegram Kind = "telegram"
	KindMAX      Kind = "max"
)

// UserRef identifies a user/chat inside a concrete messenger transport.
type UserRef struct {
	Kind   Kind
	UserID int64
	ChatID int64
}

// MessageRef identifies a previously sent transport message.
type MessageRef struct {
	Kind      Kind
	ChatID    int64
	MessageID int
}

// ActionRef identifies a transport action/callback interaction.
type ActionRef struct {
	Kind Kind
	ID   string
}

// ActionButton is a transport-neutral interactive element.
type ActionButton struct {
	Text string
	URL  string

	// Action carries an internal callback/action identifier.
	Action string
}

// OutgoingMessage is a transport-neutral outbound message.
type OutgoingMessage struct {
	Text    string
	Buttons [][]ActionButton
}

// Sender sends and edits transport messages.
type Sender interface {
	Send(ctx context.Context, user UserRef, msg OutgoingMessage) error
	Edit(ctx context.Context, ref MessageRef, msg OutgoingMessage) error
	AnswerAction(ctx context.Context, ref ActionRef, text string) error
}
