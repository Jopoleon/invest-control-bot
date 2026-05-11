package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestSendMessageIncludesChatAndRelayErrorContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/sendMessage") {
			t.Fatalf("path=%q want sendMessage", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":502,"description":"Telegram relay upstream timeout after 8000ms","error":"upstream_timeout","timeout_ms":8000}`))
	}))
	defer server.Close()

	client, err := NewClientWithOptions("123:test", "", ClientOptions{ServerURL: server.URL})
	if err != nil {
		t.Fatalf("NewClientWithOptions: %v", err)
	}

	err = client.SendMessage(context.Background(), 5606385118, "hello", nil)
	if err == nil {
		t.Fatal("expected send error")
	}
	text := err.Error()
	if !strings.Contains(text, "telegram sendMessage chat_id=5606385118") {
		t.Fatalf("error lacks chat context: %q", text)
	}
	if !strings.Contains(text, "Telegram relay upstream timeout") {
		t.Fatalf("error lacks relay root cause: %q", text)
	}
}

func TestClientGetChatMemberReturnsStatusPermissionsAndRawPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/getChatMember") {
			t.Fatalf("path=%q want getChatMember", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		if got := r.FormValue("chat_id"); got != "-1003222018503" {
			t.Fatalf("chat_id=%q", got)
		}
		if got := r.FormValue("user_id"); got != "5228612359" {
			t.Fatalf("user_id=%q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"status":"administrator","user":{"id":5228612359,"is_bot":false,"username":"vva8669"},"can_manage_chat":true,"can_invite_users":true,"can_restrict_members":true,"can_delete_messages":false,"can_manage_video_chats":false,"can_promote_members":false,"can_change_info":false,"is_anonymous":false}}`))
	}))
	defer server.Close()

	client, err := NewClientWithOptions("123:test", "", ClientOptions{ServerURL: server.URL})
	if err != nil {
		t.Fatalf("NewClientWithOptions: %v", err)
	}
	member, err := client.GetChatMember(context.Background(), "-1003222018503", 5228612359)
	if err != nil {
		t.Fatalf("GetChatMember: %v", err)
	}
	if member.Status != "administrator" || member.UserID != 5228612359 || member.Username != "vva8669" {
		t.Fatalf("member=%+v", member)
	}
	if !containsString(member.Permissions, "can_invite_users") || !containsString(member.Permissions, "can_restrict_members") {
		t.Fatalf("permissions=%v", member.Permissions)
	}
	if len(member.Raw) == 0 {
		t.Fatalf("raw payload should be preserved")
	}
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}
