package bot

import (
	"context"

	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

type sentRecord struct {
	user messenger.UserRef
	msg  messenger.OutgoingMessage
}

type editedRecord struct {
	ref messenger.MessageRef
	msg messenger.OutgoingMessage
}

type answeredRecord struct {
	ref  messenger.ActionRef
	text string
}

type fakeSender struct {
	sent     []sentRecord
	edited   []editedRecord
	answered []answeredRecord
}

func (f *fakeSender) Send(_ context.Context, user messenger.UserRef, msg messenger.OutgoingMessage) error {
	f.sent = append(f.sent, sentRecord{user: user, msg: msg})
	return nil
}

func (f *fakeSender) Edit(_ context.Context, ref messenger.MessageRef, msg messenger.OutgoingMessage) error {
	f.edited = append(f.edited, editedRecord{ref: ref, msg: msg})
	return nil
}

func (f *fakeSender) AnswerAction(_ context.Context, ref messenger.ActionRef, text string) error {
	f.answered = append(f.answered, answeredRecord{ref: ref, text: text})
	return nil
}
