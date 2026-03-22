package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestSessionsPageRenders(t *testing.T) {
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", nil, nil)
	now := time.Now().UTC()
	if err := st.CreateAdminSession(context.Background(), domain.AdminSession{
		TokenHash:  "hash1",
		Subject:    "admin",
		CreatedAt:  now,
		ExpiresAt:  now.Add(time.Hour),
		LastSeenAt: now,
		IP:         "127.0.0.1",
		UserAgent:  "test-agent",
	}); err != nil {
		t.Fatalf("CreateAdminSession: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/sessions?lang=ru", nil)
	rec := httptest.NewRecorder()
	h.sessionsPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("sessionsPage status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Активные сессии") {
		t.Fatalf("response does not contain page title")
	}
}

func TestRevokeAdminSessionMarksSessionRevoked(t *testing.T) {
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", nil, nil)
	now := time.Now().UTC()
	if err := st.CreateAdminSession(context.Background(), domain.AdminSession{
		TokenHash:  "hash2",
		Subject:    "admin",
		CreatedAt:  now,
		ExpiresAt:  now.Add(time.Hour),
		LastSeenAt: now,
		IP:         "127.0.0.1",
		UserAgent:  "test-agent",
	}); err != nil {
		t.Fatalf("CreateAdminSession: %v", err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/admin/login?lang=ru", nil)
	getRec := httptest.NewRecorder()
	h.loginPage(getRec, getReq)
	resp := getRec.Result()
	defer resp.Body.Close()
	var csrfCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == csrfCookieName {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatalf("csrf cookie missing")
	}

	form := url.Values{}
	form.Set("csrf_token", csrfCookie.Value)
	form.Set("id", "1")
	req := httptest.NewRequest(http.MethodPost, "/admin/sessions/revoke?lang=ru", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	h.revokeAdminSession(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 999}}))

	if rec.Code != http.StatusFound {
		t.Fatalf("revoke status=%d", rec.Code)
	}
	session, found, err := st.GetAdminSessionByTokenHash(context.Background(), "hash2")
	if err != nil || !found {
		t.Fatalf("GetAdminSessionByTokenHash found=%v err=%v", found, err)
	}
	if session.RevokedAt == nil {
		t.Fatalf("session was not revoked")
	}
}
