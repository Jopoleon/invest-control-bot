package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginRateLimiter_AllowFailureSuccessFlow(t *testing.T) {
	limiter := newLoginRateLimiter()
	req := httptest.NewRequest(http.MethodPost, "/admin/login", nil)
	req.RemoteAddr = "203.0.113.10:4567"

	if !limiter.Allow(req) {
		t.Fatal("expected fresh request to be allowed")
	}
	for i := 0; i < loginRateLimitMaxFailure; i++ {
		limiter.RegisterFailure(req)
	}
	if limiter.Allow(req) {
		t.Fatal("expected request to be blocked after max failures")
	}

	limiter.RegisterSuccess(req)
	if !limiter.Allow(req) {
		t.Fatal("expected request to be allowed after success reset")
	}
}

func TestLoginRateLimiter_ResetWindowAndClientIP(t *testing.T) {
	limiter := newLoginRateLimiter()
	req := httptest.NewRequest(http.MethodPost, "/admin/login", nil)
	req.RemoteAddr = "198.51.100.20:9999"

	limiter.attempts[clientIP(req)] = loginAttempt{
		count:   loginRateLimitMaxFailure,
		resetAt: time.Now().Add(-time.Minute),
	}
	if !limiter.Allow(req) {
		t.Fatal("expected expired rate-limit window to reset")
	}

	nilReqIP := clientIP(nil)
	if nilReqIP != "unknown" {
		t.Fatalf("clientIP(nil) = %q", nilReqIP)
	}
}
