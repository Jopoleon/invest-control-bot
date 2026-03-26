package max

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestPollerAdvancesMarker(t *testing.T) {
	t.Helper()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch call {
		case 1:
			if got := r.URL.Query().Get("marker"); got != "" {
				t.Fatalf("first marker = %q, want empty", got)
			}
			_, _ = w.Write([]byte(`{"updates":[{"update_type":"message_created"}],"marker":2}`))
		default:
			if got := r.URL.Query().Get("marker"); got != "2" {
				t.Fatalf("second marker = %q, want 2", got)
			}
			_, _ = w.Write([]byte(`{"updates":[],"marker":2}`))
		}
	}))
	defer server.Close()

	client := NewClient("test-token", server.Client())
	client.SetBaseURL(server.URL)
	poller := &Poller{Client: client, TimeoutSec: 1}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var handled int
	err := poller.Run(ctx, func(ctx context.Context, update Update) error {
		handled++
		cancel()
		return nil
	})
	if err != context.Canceled {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if handled != 1 {
		t.Fatalf("handled = %d, want 1", handled)
	}
}
