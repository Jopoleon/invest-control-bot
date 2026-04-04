package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestEnsureCSRFToken_ReusesValidCookie(t *testing.T) {
	h := NewHandler(memory.New(), "token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: strings.Repeat("a", csrfTokenBytes*2)})
	rec := httptest.NewRecorder()

	token := h.ensureCSRFToken(rec, req)
	if token != strings.Repeat("a", csrfTokenBytes*2) {
		t.Fatalf("token = %q", token)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("expected no new cookie to be issued")
	}
}

func TestEnsureCSRFToken_GeneratesCookieForInvalidToken(t *testing.T) {
	h := NewHandler(memory.New(), "token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/admin/login", nil)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "bad"})
	rec := httptest.NewRecorder()

	token := h.ensureCSRFToken(rec, req)
	if !isValidCSRFToken(token) {
		t.Fatalf("generated token invalid: %q", token)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != csrfCookieName {
		t.Fatalf("csrf cookie not issued: %+v", cookies)
	}
}

func TestVerifyCSRF(t *testing.T) {
	h := NewHandler(memory.New(), "token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)
	token := strings.Repeat("a", csrfTokenBytes*2)

	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader("csrf_token="+token))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	if err := req.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	if !h.verifyCSRF(req) {
		t.Fatal("verifyCSRF=false want true")
	}

	badReq := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader("csrf_token="+token))
	badReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	badReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: strings.Repeat("b", csrfTokenBytes*2)})
	if err := badReq.ParseForm(); err != nil {
		t.Fatalf("ParseForm: %v", err)
	}
	if h.verifyCSRF(badReq) {
		t.Fatal("verifyCSRF=true want false for mismatched tokens")
	}
}
