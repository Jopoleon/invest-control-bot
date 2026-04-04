package admin

import (
	"context"
	"strings"
	"testing"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
)

func TestSendViaMessengerAccount(t *testing.T) {
	maxSpy := &adminSpySender{}
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("telegram.NewClient: %v", err)
	}
	h := NewHandler(memory.New(), "token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", tg, maxSpy, nil)

	err = h.sendViaMessengerAccount(context.Background(), domain.UserMessengerAccount{
		MessengerKind:   domain.MessengerKindMAX,
		MessengerUserID: "193465776",
	}, messenger.OutgoingMessage{Text: "hello"})
	if err != nil {
		t.Fatalf("sendViaMessengerAccount MAX: %v", err)
	}
	if len(maxSpy.sent) != 1 || maxSpy.sent[0].user.Kind != messenger.KindMAX || maxSpy.sent[0].user.UserID != 193465776 {
		t.Fatalf("unexpected MAX send payload: %+v", maxSpy.sent)
	}

	err = h.sendViaMessengerAccount(context.Background(), domain.UserMessengerAccount{
		MessengerKind:   domain.MessengerKindTelegram,
		MessengerUserID: "264704572",
	}, messenger.OutgoingMessage{Text: "hello"})
	if err != nil {
		t.Fatalf("sendViaMessengerAccount Telegram: %v", err)
	}
}

func TestSendViaMessengerAccount_Errors(t *testing.T) {
	h := NewHandler(memory.New(), "token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	if err := h.sendViaMessengerAccount(context.Background(), domain.UserMessengerAccount{
		MessengerKind:   domain.MessengerKindMAX,
		MessengerUserID: "bad-id",
	}, messenger.OutgoingMessage{}); err == nil {
		t.Fatal("expected invalid id error")
	}
	if err := h.sendViaMessengerAccount(context.Background(), domain.UserMessengerAccount{
		MessengerKind:   domain.MessengerKindMAX,
		MessengerUserID: "193465776",
	}, messenger.OutgoingMessage{}); err == nil || !strings.Contains(err.Error(), "max sender is not configured") {
		t.Fatalf("unexpected MAX error: %v", err)
	}
	if err := h.sendViaMessengerAccount(context.Background(), domain.UserMessengerAccount{
		MessengerKind:   domain.MessengerKind("vk"),
		MessengerUserID: "42",
	}, messenger.OutgoingMessage{}); err == nil || !strings.Contains(err.Error(), "unsupported messenger kind") {
		t.Fatalf("unexpected unsupported-kind error: %v", err)
	}
}

func TestBuildPaymentLinkMessage(t *testing.T) {
	h := NewHandler(memory.New(), "token", "friendly_111_neighbour_bot", "id9718272494_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)
	connector := domain.Connector{Name: "MAX tariff", StartPayload: "in-paylink-max"}

	msg, ok := h.buildPaymentLinkMessage("ru", domain.UserMessengerAccount{MessengerKind: domain.MessengerKindMAX}, connector)
	if !ok {
		t.Fatal("expected MAX payment link message to be sendable")
	}
	if len(msg.Buttons) != 1 || msg.Buttons[0][0].URL != "https://max.ru/id9718272494_bot?start=in-paylink-max" {
		t.Fatalf("unexpected MAX buttons: %+v", msg.Buttons)
	}
	if !strings.Contains(msg.Text, "/start in-paylink-max") {
		t.Fatalf("MAX text does not contain fallback command: %q", msg.Text)
	}

	msg, ok = h.buildPaymentLinkMessage("ru", domain.UserMessengerAccount{MessengerKind: domain.MessengerKindTelegram}, connector)
	if !ok {
		t.Fatal("expected Telegram payment link message to be sendable")
	}
	if len(msg.Buttons) != 1 || msg.Buttons[0][0].URL != "https://t.me/friendly_111_neighbour_bot?start=in-paylink-max" {
		t.Fatalf("unexpected Telegram buttons: %+v", msg.Buttons)
	}
	if strings.Contains(msg.Text, "/start in-paylink-max") {
		t.Fatalf("Telegram text should not contain MAX fallback command: %q", msg.Text)
	}

	msg, ok = h.buildPaymentLinkMessage("ru", domain.UserMessengerAccount{MessengerKind: domain.MessengerKindTelegram}, domain.Connector{Name: "Empty"})
	if ok {
		t.Fatal("expected empty start payload to produce non-sendable payment link")
	}
	if len(msg.Buttons) != 0 {
		t.Fatalf("unexpected buttons for empty payload: %+v", msg.Buttons)
	}
}
