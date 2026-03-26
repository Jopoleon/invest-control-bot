package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
	"github.com/go-telegram/bot/models"
)

func TestHandleStart_SendsConnectorCardWithConsentButton(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	h := NewHandler(st, sender, nil, false, "http://localhost:8080", "test-encryption-key-123456789012345")

	connectorID := seedBotConnector(t, ctx, st, "in-start-card")

	h.handleStart(ctx, messengerMessage(9101, "egor", 9101, "/start in-start-card"))

	if len(sender.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.sent))
	}
	msg := sender.sent[0].msg
	if !strings.Contains(msg.Text, "test-connector") {
		t.Fatalf("text does not contain connector name: %q", msg.Text)
	}
	if !strings.Contains(msg.Text, "https://example.com/contract") {
		t.Fatalf("text does not contain fallback offer url: %q", msg.Text)
	}
	if len(msg.Buttons) != 1 || len(msg.Buttons[0]) != 1 {
		t.Fatalf("unexpected buttons layout: %+v", msg.Buttons)
	}
	if msg.Buttons[0][0].Action != "accept_terms:"+int64ToString(connectorID) {
		t.Fatalf("button action = %q", msg.Buttons[0][0].Action)
	}
}

func TestHandlePayConsentToggle_RendersRecurringEnabledState(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	h := NewHandler(st, sender, nil, true, "https://investcontrol.example", "test-encryption-key-123456789012345")

	connectorID := seedBotConnector(t, ctx, st, "in-consent-toggle")
	seedRecurringLegalDocs(t, ctx, st)

	h.handlePayConsentToggle(ctx, testAction("toggle-on", 9102, "egor", payConsentCallbackPrefix+int64ToString(connectorID)+":on"))

	if len(sender.edited) != 1 {
		t.Fatalf("edited messages = %d, want 1", len(sender.edited))
	}
	msg := sender.edited[0].msg
	if !strings.Contains(msg.Text, "Автоплатеж будет включен") {
		t.Fatalf("text = %q", msg.Text)
	}
	if len(msg.Buttons) != 2 {
		t.Fatalf("rows = %d, want 2", len(msg.Buttons))
	}
	if msg.Buttons[0][0].Action != payConsentCallbackPrefix+int64ToString(connectorID)+":off" {
		t.Fatalf("toggle action = %q", msg.Buttons[0][0].Action)
	}
	if msg.Buttons[1][0].Action != "pay:"+int64ToString(connectorID)+":1" {
		t.Fatalf("pay action = %q", msg.Buttons[1][0].Action)
	}
}

func TestShowAutopaySubscriptionChooser_BuildsPerSubscriptionActions(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	h := NewHandler(st, sender, nil, true, "https://investcontrol.example", "test-encryption-key-123456789012345")

	connectorEnabled := seedAutopayConnector(t, ctx, st, "chooser-enabled", "enabled")
	connectorReactivatable := seedAutopayConnector(t, ctx, st, "chooser-react", "react")
	connectorCheckout := seedAutopayConnector(t, ctx, st, "chooser-checkout", "checkout")

	paymentEnabled := seedPayment(t, ctx, st, 9103, connectorEnabled, true)
	paymentReactivatable := seedPayment(t, ctx, st, 9103, connectorReactivatable, true)
	paymentCheckout := seedPayment(t, ctx, st, 9103, connectorCheckout, false)

	subEnabled := seedSubscription(t, ctx, st, 9103, connectorEnabled, paymentEnabled, true)
	subReactivatable := seedSubscription(t, ctx, st, 9103, connectorReactivatable, paymentReactivatable, false)
	_ = subReactivatable
	_ = seedSubscription(t, ctx, st, 9103, connectorCheckout, paymentCheckout, false)

	h.showAutopaySubscriptionChooser(ctx, testAction("chooser", 9103, "egor", menuCallbackAutopayPick))

	if len(sender.edited) != 1 {
		t.Fatalf("edited messages = %d, want 1", len(sender.edited))
	}
	msg := sender.edited[0].msg
	if !strings.Contains(msg.Text, "Выберите подписку") {
		t.Fatalf("text = %q", msg.Text)
	}
	if len(msg.Buttons) != 4 {
		t.Fatalf("rows = %d, want 4", len(msg.Buttons))
	}
	var (
		foundDisable     bool
		foundReactivate  bool
		foundCheckout    bool
		foundBack        bool
		expectedDisable  = menuCallbackAutopayOffSub + int64ToString(subEnabled)
		expectedCheckout = "https://investcontrol.example/subscribe/" + int64ToString(connectorCheckout)
	)
	for _, row := range msg.Buttons {
		if len(row) == 0 {
			continue
		}
		button := row[0]
		switch {
		case button.Action == expectedDisable:
			foundDisable = true
		case strings.HasPrefix(button.Action, menuCallbackAutopayOnSub):
			foundReactivate = true
		case button.URL == expectedCheckout:
			foundCheckout = true
		case button.Action == menuCallbackAutopay:
			foundBack = true
		}
	}
	if !foundDisable || !foundReactivate || !foundCheckout || !foundBack {
		t.Fatalf("unexpected chooser layout: %+v", msg.Buttons)
	}
}

func TestHandleUpdate_MapsTelegramMessageAndCallbackToInternalEvents(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	sender := &fakeSender{}
	h := NewHandler(st, sender, nil, true, "http://localhost:8080", "test-encryption-key-123456789012345")

	h.HandleUpdate(ctx, &models.Update{
		Message: &models.Message{
			ID:   1,
			From: &models.User{ID: 9104, Username: "egor"},
			Chat: models.Chat{ID: 9104},
			Text: "/menu",
		},
	})

	if len(sender.sent) != 1 {
		t.Fatalf("sent messages after /menu = %d, want 1", len(sender.sent))
	}
	if sender.sent[0].msg.Text != "Личный кабинет:" {
		t.Fatalf("menu text = %q", sender.sent[0].msg.Text)
	}

	connectorID := seedBotConnector(t, ctx, st, "in-update-toggle")
	seedRecurringLegalDocs(t, ctx, st)

	h.HandleUpdate(ctx, &models.Update{
		CallbackQuery: &models.CallbackQuery{
			ID:   "cb-update",
			From: models.User{ID: 9104, Username: "egor"},
			Message: models.MaybeInaccessibleMessage{
				Type: models.MaybeInaccessibleMessageTypeMessage,
				Message: &models.Message{
					ID:   55,
					From: &models.User{ID: 9104, Username: "egor"},
					Chat: models.Chat{ID: 9104},
					Date: int(time.Now().Unix()),
					Text: "old",
				},
			},
			Data: payConsentCallbackPrefix + int64ToString(connectorID) + ":on",
		},
	})

	if len(sender.answered) != 1 {
		t.Fatalf("answered actions = %d, want 1", len(sender.answered))
	}
	if sender.answered[0].ref.ID != "cb-update" {
		t.Fatalf("callback answer id = %q", sender.answered[0].ref.ID)
	}
	if len(sender.edited) != 1 {
		t.Fatalf("edited messages after callback = %d, want 1", len(sender.edited))
	}
	if sender.edited[0].ref.MessageID != 55 {
		t.Fatalf("edited message id = %d", sender.edited[0].ref.MessageID)
	}
}
