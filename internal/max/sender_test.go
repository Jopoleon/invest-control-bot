package max

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

func TestSenderSendUsesUserIDForPrivateDialog(t *testing.T) {
	t.Helper()

	var gotUserID string
	var gotChatID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = r.URL.Query().Get("user_id")
		gotChatID = r.URL.Query().Get("chat_id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"sender":{"user_id":264704572},"recipient":{"user_id":264704572},"body":{"mid":"42","text":"ok"}}}`))
	}))
	defer server.Close()

	client := NewClient("test-token", server.Client())
	client.SetBaseURL(server.URL)
	sender := NewSender(client)

	err := sender.Send(context.Background(), messenger.UserRef{Kind: messenger.KindMAX, ChatID: 264704572}, messenger.OutgoingMessage{Text: "Привет"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotUserID != "264704572" {
		t.Fatalf("user_id = %q, want 264704572", gotUserID)
	}
	if gotChatID != "" {
		t.Fatalf("chat_id = %q, want empty", gotChatID)
	}
}

func TestSenderAnswerActionSkipsEmptyNotification(t *testing.T) {
	t.Helper()

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient("test-token", server.Client())
	client.SetBaseURL(server.URL)
	sender := NewSender(client)

	if err := sender.AnswerAction(context.Background(), messenger.ActionRef{Kind: messenger.KindMAX, ID: "cb-1"}, "   "); err != nil {
		t.Fatalf("AnswerAction: %v", err)
	}
	if called {
		t.Fatal("expected empty notification to skip POST /answers")
	}
}
