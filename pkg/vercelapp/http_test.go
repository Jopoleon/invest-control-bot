package vercelapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func resetHTTPBootstrap() {
	handlerOnce = sync.Once{}
	httpHandler = nil
	handlerErr = nil
}

func TestHTTPHandler_ReturnsBootstrapError(t *testing.T) {
	resetHTTPBootstrap()
	t.Setenv("APP_RUNTIME", "server")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	HTTPHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `APP_RUNTIME must be "vercel"`) {
		t.Fatalf("body=%q want runtime mismatch", rec.Body.String())
	}
}
