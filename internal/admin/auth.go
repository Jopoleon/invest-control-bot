package admin

import (
	"net/http"
	"strings"
)

// authorized validates admin token from query string, cookie or bearer header.
func (h *Handler) authorized(r *http.Request) bool {
	if h.adminToken == "" {
		return true
	}
	if r.URL.Query().Get("token") == h.adminToken {
		return true
	}
	if c, err := r.Cookie("admin_token"); err == nil && strings.TrimSpace(c.Value) == h.adminToken {
		return true
	}
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ") == h.adminToken
	}
	return false
}

// persistTokenCookie saves admin token from URL into HTTP-only cookie for convenience.
func (h *Handler) persistTokenCookie(w http.ResponseWriter, r *http.Request) {
	if h.adminToken == "" {
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" || token != h.adminToken {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// unauthorized writes minimal unauthorized response for admin routes.
func (h *Handler) unauthorized(w http.ResponseWriter) {
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("unauthorized"))
}
