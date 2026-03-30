package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestConnectorsPageCreate_UsesRedirectAfterPost(t *testing.T) {
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil)

	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/connectors?lang=ru", nil)
	csrfRec := httptest.NewRecorder()
	csrfToken := h.ensureCSRFToken(csrfRec, csrfReq)
	csrfResp := csrfRec.Result()
	defer csrfResp.Body.Close()

	var csrfCookie *http.Cookie
	for _, c := range csrfResp.Cookies() {
		if c.Name == csrfCookieName {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("csrf cookie missing")
	}

	form := url.Values{}
	form.Set("csrf_token", csrfToken)
	form.Set("name", "MAX Test")
	form.Set("price_rub", "3200")
	form.Set("period_days", "30")
	form.Set("channel_url", "https://web.max.ru/-72598909498032")

	req := httptest.NewRequest(http.MethodPost, "/admin/connectors?lang=ru", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	h.connectorsPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "/admin/connectors?lang=ru&notice=") {
		t.Fatalf("redirect location = %q", location)
	}

	connectors, err := st.ListConnectors(req.Context())
	if err != nil {
		t.Fatalf("ListConnectors() error = %v", err)
	}
	if got := len(connectors); got != 1 {
		t.Fatalf("connector count = %d, want 1", got)
	}
	if connectors[0].StartPayload == "" || !strings.HasPrefix(connectors[0].StartPayload, "in-") {
		t.Fatalf("generated start payload = %q, want in-*", connectors[0].StartPayload)
	}
}
