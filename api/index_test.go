package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler_DelegatesToVercelHTTPHandler(t *testing.T) {
	t.Setenv("APP_RUNTIME", "server")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `APP_RUNTIME must be "vercel"`) {
		t.Fatalf("body=%q want vercel runtime error", rec.Body.String())
	}
}
