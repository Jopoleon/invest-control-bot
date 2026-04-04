package max

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientPingCallsMeEndpoint(t *testing.T) {
	t.Helper()

	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/me" {
			t.Fatalf("path = %q, want /me", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "test-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":218306705,"first_name":"InvestControlBot","username":"id9718272494_bot","is_bot":true}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.Client())
	client.SetBaseURL(server.URL)

	info, err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if !called {
		t.Fatal("expected /me to be called")
	}
	if info.UserID != 218306705 {
		t.Fatalf("user_id = %d", info.UserID)
	}
	if info.Username != "id9718272494_bot" {
		t.Fatalf("username = %q", info.Username)
	}
}

func TestClientGetUpdatesBuildsLongPollingRequest(t *testing.T) {
	t.Helper()

	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/updates" {
			t.Fatalf("path = %q, want /updates", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "test-token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "50" {
			t.Fatalf("limit = %q", got)
		}
		if got := r.URL.Query().Get("timeout"); got != "20" {
			t.Fatalf("timeout = %q", got)
		}
		if got := r.URL.Query().Get("marker"); got != "15" {
			t.Fatalf("marker = %q", got)
		}
		if got := r.URL.Query().Get("types"); got != "message_created,message_callback" {
			t.Fatalf("types = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"updates":[{"update_type":"message_created","timestamp":1}],"marker":16}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.Client())
	client.SetBaseURL(server.URL)

	marker := int64(15)
	page, err := client.GetUpdates(context.Background(), GetUpdatesRequest{
		Limit:      50,
		TimeoutSec: 20,
		Marker:     &marker,
		Types:      []string{"message_created", "message_callback"},
	})
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if !called {
		t.Fatal("expected test server to be called")
	}
	if len(page.Updates) != 1 || page.Updates[0].UpdateType != "message_created" {
		t.Fatalf("unexpected updates page: %+v", page)
	}
	if page.Marker == nil || *page.Marker != 16 {
		t.Fatalf("marker = %v, want 16", page.Marker)
	}
}

func TestClientSendMessageBuildsInlineKeyboardRequest(t *testing.T) {
	t.Helper()

	var body newMessageBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("path = %q, want /messages", r.URL.Path)
		}
		if got := r.URL.Query().Get("user_id"); got != "264704572" {
			t.Fatalf("user_id = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "test-token" {
			t.Fatalf("authorization = %q", got)
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("content-type = %q", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"sender":{"user_id":264704572},"recipient":{"user_id":264704572},"body":{"mid":"42","text":"ok"}}}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.Client())
	client.SetBaseURL(server.URL)

	message, err := client.SendMessage(context.Background(), SendMessageRequest{
		UserID: 264704572,
		Text:   "Привет",
		Attachments: []Attachment{{
			Type: "inline_keyboard",
			Payload: InlineKeyboardPayload{
				Buttons: [][]InlineKeyboardButton{{
					{Type: "link", Text: "Открыть", URL: "https://example.com"},
				}},
			},
		}},
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if body.Text != "Привет" {
		t.Fatalf("text = %q", body.Text)
	}
	if len(body.Attachments) != 1 || body.Attachments[0].Type != "inline_keyboard" {
		t.Fatalf("unexpected attachments: %+v", body.Attachments)
	}
	if message.Body == nil || message.Body.MID != "42" {
		t.Fatalf("message body = %+v, want mid=42", message.Body)
	}
	if message.Sender == nil || message.Sender.UserID != 264704572 {
		t.Fatalf("sender = %+v", message.Sender)
	}
	if message.MessageID != 0 {
		t.Fatalf("legacy message_id = %d, want 0", message.MessageID)
	}
}

func TestClientEnsureWebhookDeletesStaleAndCreatesDesired(t *testing.T) {
	t.Helper()

	var deletedURL string
	var createdBody CreateWebhookSubscriptionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/subscriptions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"subscriptions":[{"url":"https://old.example/max/webhook"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/subscriptions":
			deletedURL = r.URL.Query().Get("url")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/subscriptions":
			if err := json.NewDecoder(r.Body).Decode(&createdBody); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClient("test-token", server.Client())
	client.SetBaseURL(server.URL)

	err := client.EnsureWebhook(context.Background(), "https://new.example/max/webhook", "test-secret", []string{"message_created", "message_callback"})
	if err != nil {
		t.Fatalf("EnsureWebhook: %v", err)
	}
	if deletedURL != "https://old.example/max/webhook" {
		t.Fatalf("deleted url = %q", deletedURL)
	}
	if createdBody.URL != "https://new.example/max/webhook" {
		t.Fatalf("created url = %q", createdBody.URL)
	}
	if createdBody.Secret != "test-secret" {
		t.Fatalf("created secret = %q", createdBody.Secret)
	}
	if got := strings.Join(createdBody.UpdateTypes, ","); got != "message_created,message_callback" {
		t.Fatalf("created update types = %q", got)
	}
}

func TestClientAddChatMembersBuildsRequest(t *testing.T) {
	t.Helper()

	var body map[string][]int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/chats/-72598909498032/members" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.Client())
	client.SetBaseURL(server.URL)

	if err := client.AddChatMembers(context.Background(), -72598909498032, []int64{193465776}); err != nil {
		t.Fatalf("AddChatMembers: %v", err)
	}
	if got := body["user_ids"]; len(got) != 1 || got[0] != 193465776 {
		t.Fatalf("user_ids = %+v, want [193465776]", got)
	}
}

func TestClientAddChatMembers_ReturnsErrorOnPartialFailure(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chats/-72598909498032/members" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"failed_user_ids":[193465776],"message":"user already in chat"}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.Client())
	client.SetBaseURL(server.URL)

	err := client.AddChatMembers(context.Background(), -72598909498032, []int64{193465776})
	if err == nil || !strings.Contains(err.Error(), "partial failure") {
		t.Fatalf("AddChatMembers err=%v want partial failure", err)
	}
	if !strings.Contains(err.Error(), "failed_user_ids=193465776") {
		t.Fatalf("AddChatMembers err=%v want failed_user_ids", err)
	}
	if !strings.Contains(err.Error(), "user already in chat") {
		t.Fatalf("AddChatMembers err=%v want message", err)
	}
}

func TestClientAddChatMembers_ReturnsVerboseErrorOnSuccessFalseWithFailedDetails(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chats/-72598909498032/members" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"message":"cannot add member","failed_user_ids":[193465776],"failed_user_details":[{"user_id":193465776,"code":"already_member","message":"user already in chat"}]}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.Client())
	client.SetBaseURL(server.URL)

	err := client.AddChatMembers(context.Background(), -72598909498032, []int64{193465776})
	if err == nil || !strings.Contains(err.Error(), "cannot add member") {
		t.Fatalf("AddChatMembers err=%v want verbose message", err)
	}
	if !strings.Contains(err.Error(), "already_member") {
		t.Fatalf("AddChatMembers err=%v want failed detail code", err)
	}
	if !strings.Contains(err.Error(), "failed_user_ids=193465776") {
		t.Fatalf("AddChatMembers err=%v want failed_user_ids", err)
	}
}

func TestClientRemoveChatMemberBuildsRequestWithoutBlock(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/chats/-72598909498032/members" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("user_id"); got != "193465776" {
			t.Fatalf("user_id = %q", got)
		}
		if got := r.URL.Query().Get("block"); got != "" {
			t.Fatalf("block = %q, want empty", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.Client())
	client.SetBaseURL(server.URL)

	if err := client.RemoveChatMember(context.Background(), -72598909498032, 193465776, false); err != nil {
		t.Fatalf("RemoveChatMember: %v", err)
	}
}

func TestClientGetMyChatMemberCallsMembershipEndpoint(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/chats/-72598909498032/members/me" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":218306705,"is_admin":true,"permissions":["add_remove_members"]}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.Client())
	client.SetBaseURL(server.URL)

	member, err := client.GetMyChatMember(context.Background(), -72598909498032)
	if err != nil {
		t.Fatalf("GetMyChatMember: %v", err)
	}
	if member.UserID != 218306705 || !member.IsAdmin || len(member.Permissions) != 1 || member.Permissions[0] != "add_remove_members" {
		t.Fatalf("member = %+v", member)
	}
}
