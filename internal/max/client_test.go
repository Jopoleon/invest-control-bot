package max

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
