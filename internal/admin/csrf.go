package admin

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

const (
	csrfCookieName = "admin_csrf"
	csrfTokenBytes = 32
	csrfCookieTTL  = 8 * time.Hour
)

func (h *Handler) ensureCSRFToken(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(csrfCookieName); err == nil {
		token := strings.TrimSpace(c.Value)
		if isValidCSRFToken(token) {
			return token
		}
	}
	token := randomHex(csrfTokenBytes)
	h.setCookie(w, r, http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(csrfCookieTTL.Seconds()),
	})
	return token
}

func (h *Handler) verifyCSRF(r *http.Request) bool {
	formToken := strings.TrimSpace(r.FormValue("csrf_token"))
	if !isValidCSRFToken(formToken) {
		return false
	}
	c, err := r.Cookie(csrfCookieName)
	if err != nil {
		return false
	}
	cookieToken := strings.TrimSpace(c.Value)
	if !isValidCSRFToken(cookieToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(formToken), []byte(cookieToken)) == 1
}

func randomHex(size int) string {
	if size <= 0 {
		size = 16
	}
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "fallback-csrf-token"
	}
	return hex.EncodeToString(raw)
}

func isValidCSRFToken(token string) bool {
	if len(token) != csrfTokenBytes*2 {
		return false
	}
	for _, ch := range token {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}
