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

func TestLoginCreatesServerSessionAndAllowsProtectedPage(t *testing.T) {
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil)

	loginGet := httptest.NewRequest(http.MethodGet, "/admin/login?lang=ru", nil)
	loginGetRec := httptest.NewRecorder()
	h.loginPage(loginGetRec, loginGet)
	loginResp := loginGetRec.Result()
	defer loginResp.Body.Close()

	var csrfCookie *http.Cookie
	for _, c := range loginResp.Cookies() {
		if c.Name == csrfCookieName {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatalf("csrf cookie not issued")
	}

	form := url.Values{}
	form.Set("csrf_token", csrfCookie.Value)
	form.Set("token", "test-admin-token")
	form.Set("next", "/admin/connectors")

	loginPost := httptest.NewRequest(http.MethodPost, "/admin/login?lang=ru", strings.NewReader(form.Encode()))
	loginPost.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginPost.AddCookie(csrfCookie)
	loginPostRec := httptest.NewRecorder()
	h.loginPage(loginPostRec, loginPost)
	postResp := loginPostRec.Result()
	defer postResp.Body.Close()

	if postResp.StatusCode != http.StatusFound {
		t.Fatalf("login status = %d, want %d", postResp.StatusCode, http.StatusFound)
	}

	var sessionCookie *http.Cookie
	for _, c := range postResp.Cookies() {
		if c.Name == adminSessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil || strings.TrimSpace(sessionCookie.Value) == "" {
		t.Fatalf("admin session cookie not issued")
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/connectors?lang=ru", nil)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	h.connectorsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("protected page status = %d, want 200", rec.Code)
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	rec := httptest.NewRecorder()
	if err := h.createAdminSession(rec, req); err != nil {
		t.Fatalf("createAdminSession() error = %v", err)
	}
	resp := rec.Result()
	defer resp.Body.Close()

	var sessionCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == adminSessionCookieName {
			sessionCookie = c
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("session cookie missing")
	}

	logoutReq := httptest.NewRequest(http.MethodGet, "/admin/logout", nil)
	logoutReq.AddCookie(sessionCookie)
	logoutRec := httptest.NewRecorder()
	h.logout(logoutRec, logoutReq)

	protectedReq := httptest.NewRequest(http.MethodGet, "/admin/connectors", nil)
	protectedReq.AddCookie(sessionCookie)
	protectedRec := httptest.NewRecorder()
	ok := h.requireAuth(protectedRec, protectedReq)
	if ok {
		t.Fatalf("requireAuth() = true after logout, want false")
	}
}

func TestRegisterProtectsAdminRoutesWithMiddleware(t *testing.T) {
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil)

	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/admin/connectors?lang=ru", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("middleware status = %d, want %d", rec.Code, http.StatusFound)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "/admin/login?next=") {
		t.Fatalf("redirect location = %q", location)
	}
}

func TestCreateAdminSession_CleansExpiredAndRevokedSessions(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil)
	now := time.Now().UTC()

	if err := st.CreateAdminSession(ctx, domain.AdminSession{
		TokenHash:  "expired-hash",
		Subject:    "admin",
		CreatedAt:  now.Add(-10 * time.Hour),
		ExpiresAt:  now.Add(-2 * time.Hour),
		LastSeenAt: now.Add(-3 * time.Hour),
	}); err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	revokedAt := now.Add(-time.Hour)
	if err := st.CreateAdminSession(ctx, domain.AdminSession{
		TokenHash:  "revoked-hash",
		Subject:    "admin",
		CreatedAt:  now.Add(-4 * time.Hour),
		ExpiresAt:  now.Add(4 * time.Hour),
		LastSeenAt: now.Add(-time.Hour),
		RevokedAt:  &revokedAt,
	}); err != nil {
		t.Fatalf("create revoked session: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	rec := httptest.NewRecorder()
	if err := h.createAdminSession(rec, req); err != nil {
		t.Fatalf("createAdminSession() error = %v", err)
	}

	if _, found, err := st.GetAdminSessionByTokenHash(ctx, "expired-hash"); err != nil {
		t.Fatalf("get expired session: %v", err)
	} else if found {
		t.Fatalf("expired session was not cleaned")
	}
	if _, found, err := st.GetAdminSessionByTokenHash(ctx, "revoked-hash"); err != nil {
		t.Fatalf("get revoked session: %v", err)
	} else if found {
		t.Fatalf("revoked session was not cleaned")
	}
}
