package max

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/bot"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

type fakeSender struct {
	sent     []messenger.OutgoingMessage
	edited   []messenger.OutgoingMessage
	answered []messenger.ActionRef
}

func (f *fakeSender) Send(_ context.Context, _ messenger.UserRef, msg messenger.OutgoingMessage) error {
	f.sent = append(f.sent, msg)
	return nil
}

func (f *fakeSender) Edit(_ context.Context, _ messenger.MessageRef, msg messenger.OutgoingMessage) error {
	f.edited = append(f.edited, msg)
	return nil
}

func (f *fakeSender) AnswerAction(_ context.Context, ref messenger.ActionRef, _ string) error {
	f.answered = append(f.answered, ref)
	return nil
}

func TestAdapterDispatchesBotStartedAsStartMessage(t *testing.T) {
	st := memory.New()
	sender := &fakeSender{}
	handler := bot.NewHandler(st, sender, nil, false, "http://localhost:8080", "test-encryption-key-123456789012345")
	adapter := NewAdapter(handler)
	if err := st.CreateConnector(context.Background(), domain.Connector{
		ID:           11,
		StartPayload: "in-123",
		Name:         "Тестовый тариф",
		Description:  "Описание",
		PriceRUB:     2300,
		OfferURL:     "https://example.com/oferta",
		PrivacyURL:   "https://example.com/policy",
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}

	adapter.Dispatch(context.Background(), Update{
		UpdateType:   "bot_started",
		ChatID:       264704572,
		StartPayload: "in-123",
		User:         &User{UserID: 264704572, Username: "egor"},
	})

	if len(sender.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.sent))
	}
	if !strings.Contains(sender.sent[0].Text, "Чтобы продолжить") {
		t.Fatalf("unexpected start text: %q", sender.sent[0].Text)
	}
}

func TestMapBotStartedUsesMessageSenderAndStartCommandText(t *testing.T) {
	var update Update
	if err := json.Unmarshal([]byte(`{
		"update_type":"bot_started",
		"message":{
			"sender":{"user_id":193465776,"username":"egor"},
			"recipient":{"chat_id":109778209,"chat_type":"dialog","user_id":218306705},
			"body":{"mid":"123","text":"/start in-123"}
		}
	}`), &update); err != nil {
		t.Fatalf("unmarshal update: %v", err)
	}

	msg, ok := mapBotStarted(update)
	if !ok {
		t.Fatalf("mapBotStarted returned ok=false, raw=%s", string(update.Raw))
	}
	if msg.User.ID != 193465776 {
		t.Fatalf("user id = %d, want 193465776", msg.User.ID)
	}
	if msg.ChatID != 193465776 {
		t.Fatalf("chat id = %d, want 193465776", msg.ChatID)
	}
	if msg.Text != "/start in-123" {
		t.Fatalf("text = %q, want /start in-123", msg.Text)
	}
}

func TestAdapterDispatchesMessageCallbackAsAction(t *testing.T) {
	st := memory.New()
	sender := &fakeSender{}
	handler := bot.NewHandler(st, sender, nil, true, "http://localhost:8080", "test-encryption-key-123456789012345")
	adapter := NewAdapter(handler)

	adapter.Dispatch(context.Background(), Update{
		UpdateType: "message_callback",
		Callback: &Callback{
			CallbackID: "cb-1",
			User:       &User{UserID: 264704572, Username: "egor"},
			Message:    &Message{Recipient: &Recipient{UserID: 264704572}, Body: &MessageBody{MID: "55"}},
			Payload:    "menu:autopay",
		},
	})

	if len(sender.answered) != 1 {
		t.Fatalf("answered actions = %d, want 1", len(sender.answered))
	}
	if sender.answered[0].ID != "cb-1" {
		t.Fatalf("callback id = %q", sender.answered[0].ID)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.sent))
	}
	if !strings.Contains(sender.sent[0].Text, "Автоплатеж") {
		t.Fatalf("unexpected sent text: %q", sender.sent[0].Text)
	}
}

func TestMapIncomingActionUsesTopLevelMessageWhenCallbackMessageIsMissing(t *testing.T) {
	var update Update
	if err := json.Unmarshal([]byte(`{
		"update_type":"message_callback",
		"callback":{
			"callback_id":"cb-1",
			"user":{"user_id":193465776,"username":"egor"},
			"payload":"menu:subscription"
		},
		"message":{
			"sender":{"user_id":218306705,"username":"bot"},
			"recipient":{"chat_id":109778209,"chat_type":"dialog","user_id":193465776},
			"body":{"mid":"12345","text":"Личный кабинет:"}
		}
	}`), &update); err != nil {
		t.Fatalf("unmarshal update: %v", err)
	}

	action, ok := mapIncomingAction(update)
	if !ok {
		t.Fatalf("mapIncomingAction returned ok=false, raw=%s", string(update.Raw))
	}
	if action.User.ID != 193465776 {
		t.Fatalf("user id = %d, want 193465776", action.User.ID)
	}
	if action.ChatID != 193465776 {
		t.Fatalf("chat id = %d, want sender user id 193465776", action.ChatID)
	}
	if action.MessageID != 12345 {
		t.Fatalf("message id = %d, want 12345", action.MessageID)
	}
	if action.Data != "menu:subscription" {
		t.Fatalf("data = %q", action.Data)
	}
}

func TestMapIncomingMessageUsesSenderRecipientAndBodyText(t *testing.T) {
	var update Update
	if err := json.Unmarshal([]byte(`{
		"update_type":"message_created",
		"message":{
			"sender":{"user_id":264704572,"username":"egor"},
			"recipient":{"user_id":264704572},
			"body":{"mid":"123","text":"  /menu  "}
		}
	}`), &update); err != nil {
		t.Fatalf("unmarshal update: %v", err)
	}

	msg, ok := mapIncomingMessage(update)
	if !ok {
		t.Fatalf("mapIncomingMessage returned ok=false, raw=%s", string(update.Raw))
	}
	if msg.User.ID != 264704572 {
		t.Fatalf("user id = %d, want 264704572", msg.User.ID)
	}
	if msg.ChatID != 264704572 {
		t.Fatalf("chat id = %d, want 264704572", msg.ChatID)
	}
	if msg.Text != "/menu" {
		t.Fatalf("text = %q, want /menu", msg.Text)
	}
}

func TestMapIncomingMessageUsesSenderAsDialogTargetForPrivateDM(t *testing.T) {
	var update Update
	if err := json.Unmarshal([]byte(`{
		"update_type":"message_created",
		"message":{
			"sender":{"user_id":193465776,"username":"egor"},
			"recipient":{"user_id":109778209,"chat_id":998877,"chat_type":"dialog","type":"user"},
			"body":{"mid":"123","text":"/menu"}
		}
	}`), &update); err != nil {
		t.Fatalf("unmarshal update: %v", err)
	}

	msg, ok := mapIncomingMessage(update)
	if !ok {
		t.Fatalf("mapIncomingMessage returned ok=false, raw=%s", string(update.Raw))
	}
	if msg.User.ID != 193465776 {
		t.Fatalf("user id = %d, want 193465776", msg.User.ID)
	}
	if msg.ChatID != 193465776 {
		t.Fatalf("chat id = %d, want sender user id 193465776", msg.ChatID)
	}
}
