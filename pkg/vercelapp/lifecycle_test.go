package vercelapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsAllowedCronRequest(t *testing.T) {
	t.Setenv("VERCEL_CRON_SECRET", "secret-token")

	req := httptest.NewRequest(http.MethodGet, "/api/cron/lifecycle?token=secret-token", nil)
	if !isAllowedCronRequest(req) {
		t.Fatalf("token auth should be allowed")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/cron/lifecycle", nil)
	req.Header.Set("User-Agent", "vercel-cron/1.0")
	if !isAllowedCronRequest(req) {
		t.Fatalf("vercel cron user-agent should be allowed")
	}
}

func TestLifecycleHandler_RejectsWrongMethodAndForbidden(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/cron/lifecycle", nil)
	rec := httptest.NewRecorder()
	LifecycleHandler(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want 405", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/cron/lifecycle", nil)
	rec = httptest.NewRecorder()
	LifecycleHandler(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", rec.Code)
	}
}

func TestLifecycleHandler_ReturnsRuntimeMismatch(t *testing.T) {
	t.Setenv("APP_RUNTIME", "server")

	req := httptest.NewRequest(http.MethodGet, "/api/cron/lifecycle", nil)
	req.Header.Set("User-Agent", "vercel-cron/1.0")
	rec := httptest.NewRecorder()
	LifecycleHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `APP_RUNTIME must be "vercel"`) {
		t.Fatalf("body=%q want runtime mismatch", rec.Body.String())
	}
}
