package telegram

import (
	"context"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

func TestClient_DisabledModeSkipsNetworkCalls(t *testing.T) {
	client, err := NewClient("", "")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client == nil || client.Enabled() {
		t.Fatalf("disabled client expected")
	}
	if _, err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if err := client.SendMessage(context.Background(), 10, "hello", nil); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if err := client.Send(context.Background(), messenger.UserRef{ChatID: 10}, messenger.OutgoingMessage{Text: "hello"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := client.EditMessageText(context.Background(), 10, 11, "edit", nil); err != nil {
		t.Fatalf("EditMessageText: %v", err)
	}
	if err := client.Edit(context.Background(), messenger.MessageRef{ChatID: 10, MessageID: 11}, messenger.OutgoingMessage{Text: "edit"}); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if err := client.AnswerCallbackQuery(context.Background(), "cb-1"); err != nil {
		t.Fatalf("AnswerCallbackQuery: %v", err)
	}
	if err := client.AnswerAction(context.Background(), messenger.ActionRef{ID: "cb-2"}, "ok"); err != nil {
		t.Fatalf("AnswerAction: %v", err)
	}
	if err := client.EnsureWebhook(context.Background(), "https://example.test/telegram", "secret"); err != nil {
		t.Fatalf("EnsureWebhook: %v", err)
	}
	if err := client.EnsureDefaultMenu(context.Background()); err != nil {
		t.Fatalf("EnsureDefaultMenu: %v", err)
	}
	if _, err := client.ResolveChat(context.Background(), "@test_channel"); err != nil {
		t.Fatalf("ResolveChat: %v", err)
	}
	if err := client.RemoveChatMember(context.Background(), "@test_channel", 123); err != nil {
		t.Fatalf("RemoveChatMember: %v", err)
	}
	link, err := client.CreateSingleUseInviteLink(context.Background(), "@test_channel", "one-shot", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSingleUseInviteLink: %v", err)
	}
	if link != "" {
		t.Fatalf("invite link=%q want empty for disabled client", link)
	}
	if err := client.RevokeInviteLink(context.Background(), "@test_channel", "https://t.me/+test"); err != nil {
		t.Fatalf("RevokeInviteLink: %v", err)
	}
}

func TestToTelegramKeyboard(t *testing.T) {
	keyboard := toTelegramKeyboard([][]messenger.ActionButton{{
		{Text: "Pay", URL: "https://example.test/pay"},
		{Text: "Act", Action: "menu:pay"},
	}})
	if keyboard == nil || len(keyboard.InlineKeyboard) != 1 || len(keyboard.InlineKeyboard[0]) != 2 {
		t.Fatalf("keyboard=%+v want 1x2", keyboard)
	}
	if keyboard.InlineKeyboard[0][0].URL != "https://example.test/pay" {
		t.Fatalf("URL button mismatch: %+v", keyboard.InlineKeyboard[0][0])
	}
	if keyboard.InlineKeyboard[0][1].CallbackData != "menu:pay" {
		t.Fatalf("Callback button mismatch: %+v", keyboard.InlineKeyboard[0][1])
	}
	if toTelegramKeyboard(nil) != nil {
		t.Fatalf("nil rows should return nil keyboard")
	}
}

func TestDefaultBotCommands(t *testing.T) {
	commands := defaultBotCommands()
	if len(commands) != 3 {
		t.Fatalf("commands=%d want 3", len(commands))
	}
	if commands[0].Command != "start" || commands[1].Command != "menu" || commands[2].Command != "help" {
		t.Fatalf("commands=%+v unexpected order", commands)
	}
}

func TestNewClientWithOptions_RejectsInvalidServerURL(t *testing.T) {
	if _, err := NewClientWithOptions("token", "", ClientOptions{ServerURL: "://bad"}); err == nil {
		t.Fatal("expected invalid server url error")
	}
}

func TestNewClientWithOptions_RejectsInvalidHTTPProxyURL(t *testing.T) {
	if _, err := NewClientWithOptions("token", "", ClientOptions{HTTPProxyURL: "bad-proxy"}); err == nil {
		t.Fatal("expected invalid proxy url error")
	}
}

func TestNewClientWithOptions_DisabledModeIgnoresRelaySettings(t *testing.T) {
	client, err := NewClientWithOptions("", "", ClientOptions{
		ServerURL:    "https://telegram-relay.example.com",
		HTTPProxyURL: "http://proxy.example.com:8080",
	})
	if err != nil {
		t.Fatalf("NewClientWithOptions: %v", err)
	}
	if client == nil || client.Enabled() {
		t.Fatalf("disabled client expected")
	}
}
