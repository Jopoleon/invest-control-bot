package admin

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

const adminTokenCookieName = "admin_token"
const adminTokenCookieTTL = 8 * time.Hour

// authorized validates admin token from cookie or bearer header.
func (h *Handler) authorized(r *http.Request) bool {
	if h.adminToken == "" {
		return true
	}
	if c, err := r.Cookie(adminTokenCookieName); err == nil && strings.TrimSpace(c.Value) == h.adminToken {
		return true
	}
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ") == h.adminToken
	}
	return false
}

func (h *Handler) setAdminTokenCookie(w http.ResponseWriter, r *http.Request, token string) {
	h.setCookie(w, r, http.Cookie{
		Name:     adminTokenCookieName,
		Value:    strings.TrimSpace(token),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(adminTokenCookieTTL.Seconds()),
	})
}

func (h *Handler) clearAdminTokenCookie(w http.ResponseWriter, r *http.Request) {
	h.setCookie(w, r, http.Cookie{
		Name:     adminTokenCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (h *Handler) setCookie(w http.ResponseWriter, r *http.Request, c http.Cookie) {
	c.Secure = shouldUseSecureCookies(r)
	http.SetCookie(w, &c)
}

func shouldUseSecureCookies(r *http.Request) bool {
	if r != nil && r.TLS != nil {
		return true
	}
	if r == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func (h *Handler) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if h.authorized(r) {
		return true
	}
	next := r.URL.Path
	if r.URL.RawQuery != "" {
		next += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, "/admin/login?next="+url.QueryEscape(next), http.StatusFound)
	return false
}

// unauthorized writes minimal unauthorized response for admin routes.
func (h *Handler) unauthorized(w http.ResponseWriter) {
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("unauthorized"))
}
