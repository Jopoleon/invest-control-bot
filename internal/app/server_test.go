package app

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestServerRun_StopsCleanlyOnContextCancel(t *testing.T) {
	t.Parallel()

	srv := &Server{
		httpServer: &http.Server{
			Addr:              "127.0.0.1:0",
			Handler:           http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
			ReadHeaderTimeout: time.Second,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- srv.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned error after context cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after context cancel")
	}
}
