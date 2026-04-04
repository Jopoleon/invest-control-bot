package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCronHandler_DelegatesToLifecycleHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/cron/lifecycle", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want 405", rec.Code)
	}
}
