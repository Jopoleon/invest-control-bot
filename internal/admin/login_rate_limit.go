package admin

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	loginRateLimitWindow     = 10 * time.Minute
	loginRateLimitMaxFailure = 10
)

type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
}

type loginAttempt struct {
	count   int
	resetAt time.Time
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{attempts: make(map[string]loginAttempt)}
}

func (l *loginRateLimiter) Allow(r *http.Request) bool {
	key := clientIP(r)
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	a, ok := l.attempts[key]
	if !ok || now.After(a.resetAt) {
		l.attempts[key] = loginAttempt{count: 0, resetAt: now.Add(loginRateLimitWindow)}
		return true
	}
	return a.count < loginRateLimitMaxFailure
}

func (l *loginRateLimiter) RegisterFailure(r *http.Request) {
	key := clientIP(r)
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	a, ok := l.attempts[key]
	if !ok || now.After(a.resetAt) {
		l.attempts[key] = loginAttempt{count: 1, resetAt: now.Add(loginRateLimitWindow)}
		return
	}
	a.count++
	l.attempts[key] = a
}

func (l *loginRateLimiter) RegisterSuccess(r *http.Request) {
	key := clientIP(r)
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}

func clientIP(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
