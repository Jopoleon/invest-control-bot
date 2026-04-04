package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

const (
	adminSessionCookieName     = "admin_session"
	adminSessionAbsoluteTTL    = 8 * time.Hour
	adminSessionIdleTTL        = 2 * time.Hour
	adminSessionRotationWindow = 30 * time.Minute
	adminSessionSubject        = "admin"
	adminLegacyTokenCookieName = "admin_token"
)

type adminAuthContextKey struct{}
type adminAuthorizedSessionContextKey struct{}

type authorizedSession struct {
	session   domain.AdminSession
	rawToken  string
	rotatedTo string
}

// authorized validates admin browser session or bearer token.
func (h *Handler) authorized(w http.ResponseWriter, r *http.Request) bool {
	ok, _ := h.authorizedSession(w, r)
	return ok
}

func (h *Handler) authorizedSession(w http.ResponseWriter, r *http.Request) (bool, *authorizedSession) {
	if h.adminToken == "" {
		return true, nil
	}
	if h.authorizedBearerToken(r) {
		return true, nil
	}

	c, err := r.Cookie(adminSessionCookieName)
	if err != nil {
		h.clearLegacyAdminTokenCookie(w, r)
		return false, nil
	}
	rawToken := strings.TrimSpace(c.Value)
	if rawToken == "" {
		h.clearAdminSessionCookie(w, r)
		h.clearLegacyAdminTokenCookie(w, r)
		return false, nil
	}

	tokenHash := hashSessionToken(rawToken)
	session, found, err := h.store.GetAdminSessionByTokenHash(r.Context(), tokenHash)
	if err != nil {
		slog.Error("load admin session failed", "error", err)
		h.clearAdminSessionCookie(w, r)
		h.clearLegacyAdminTokenCookie(w, r)
		return false, nil
	}
	if !found {
		h.clearAdminSessionCookie(w, r)
		h.clearLegacyAdminTokenCookie(w, r)
		return false, nil
	}

	now := time.Now().UTC()
	if session.RevokedAt != nil || now.After(session.ExpiresAt) || now.After(session.LastSeenAt.Add(adminSessionIdleTTL)) {
		if err := h.store.RevokeAdminSession(r.Context(), session.ID, now); err != nil {
			slog.Error("revoke expired admin session failed", "error", err, "session_id", session.ID)
		}
		h.logAdminAudit(r, domain.AuditActionAdminSessionRevoked, "session_expired")
		h.clearAdminSessionCookie(w, r)
		h.clearLegacyAdminTokenCookie(w, r)
		return false, nil
	}

	auth := &authorizedSession{session: session, rawToken: rawToken}
	if now.Sub(session.LastSeenAt) >= adminSessionRotationWindow {
		newToken := generateSessionToken()
		if err := h.store.RotateAdminSession(r.Context(), session.ID, hashSessionToken(newToken), now); err == nil {
			auth.rotatedTo = newToken
			auth.rawToken = newToken
			session.TokenHash = hashSessionToken(newToken)
		} else {
			slog.Error("rotate admin session failed", "error", err, "session_id", session.ID)
		}
	} else {
		if err := h.store.TouchAdminSession(r.Context(), session.ID, now); err != nil {
			slog.Error("touch admin session failed", "error", err, "session_id", session.ID)
		}
	}
	if auth.rotatedTo != "" {
		h.setAdminSessionCookie(w, r, auth.rotatedTo)
	} else {
		h.setAdminSessionCookie(w, r, auth.rawToken)
	}
	h.clearLegacyAdminTokenCookie(w, r)
	return true, auth
}

func (h *Handler) authorizedBearerToken(r *http.Request) bool {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ") == h.adminToken
	}
	if strings.TrimSpace(r.Header.Get("X-Admin-Token")) != "" {
		return strings.TrimSpace(r.Header.Get("X-Admin-Token")) == h.adminToken
	}
	return false
}

func (h *Handler) createAdminSession(w http.ResponseWriter, r *http.Request) error {
	now := time.Now().UTC()
	if err := h.store.CleanupAdminSessions(r.Context(), now); err != nil {
		slog.Error("cleanup admin sessions failed", "error", err)
	}
	rawToken := generateSessionToken()
	if err := h.store.CreateAdminSession(r.Context(), domain.AdminSession{
		TokenHash:  hashSessionToken(rawToken),
		Subject:    adminSessionSubject,
		CreatedAt:  now,
		ExpiresAt:  now.Add(adminSessionAbsoluteTTL),
		LastSeenAt: now,
		IP:         clientIP(r),
		UserAgent:  trimForStorage(r.UserAgent(), 512),
	}); err != nil {
		return err
	}
	h.setAdminSessionCookie(w, r, rawToken)
	h.clearLegacyAdminTokenCookie(w, r)
	return nil
}

func (h *Handler) revokeCurrentAdminSession(w http.ResponseWriter, r *http.Request) {
	if ok, auth := h.authorizedSession(w, r); ok && auth != nil {
		if err := h.store.RevokeAdminSession(r.Context(), auth.session.ID, time.Now().UTC()); err != nil {
			slog.Error("revoke current admin session failed", "error", err, "session_id", auth.session.ID)
		}
		h.logAdminAudit(r, domain.AuditActionAdminSessionRevoked, "session_logout")
	}
	h.clearAdminSessionCookie(w, r)
	h.clearLegacyAdminTokenCookie(w, r)
}

func (h *Handler) setAdminSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	h.setCookie(w, r, http.Cookie{
		Name:     adminSessionCookieName,
		Value:    strings.TrimSpace(token),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(adminSessionAbsoluteTTL.Seconds()),
	})
}

func (h *Handler) clearAdminSessionCookie(w http.ResponseWriter, r *http.Request) {
	h.setCookie(w, r, http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (h *Handler) clearLegacyAdminTokenCookie(w http.ResponseWriter, r *http.Request) {
	h.setCookie(w, r, http.Cookie{
		Name:     adminLegacyTokenCookieName,
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

func (h *Handler) withAdminSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.adminToken == "" {
			next.ServeHTTP(w, withAdminAuthorized(r, nil))
			return
		}
		if ok, auth := h.authorizedSession(w, r); ok {
			next.ServeHTTP(w, withAdminAuthorized(r, auth))
			return
		}
		h.redirectToLogin(w, r)
	})
}

func (h *Handler) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if isAdminAuthorized(r) {
		return true
	}
	if h.authorized(w, r) {
		return true
	}
	h.redirectToLogin(w, r)
	return false
}

func (h *Handler) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	next := r.URL.Path
	if r.URL.RawQuery != "" {
		next += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, "/admin/login?next="+url.QueryEscape(next), http.StatusFound)
}

// unauthorized writes minimal unauthorized response for admin routes.
func (h *Handler) unauthorized(w http.ResponseWriter) {
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("unauthorized"))
}

func generateSessionToken() string {
	return randomHex(32)
}

func hashSessionToken(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:])
}

func trimForStorage(value string, max int) string {
	value = strings.TrimSpace(value)
	if max > 0 && len(value) > max {
		return value[:max]
	}
	return value
}

func withAdminAuthorized(r *http.Request, auth *authorizedSession) *http.Request {
	ctx := context.WithValue(r.Context(), adminAuthContextKey{}, true)
	if auth != nil {
		ctx = context.WithValue(ctx, adminAuthorizedSessionContextKey{}, auth)
	}
	return r.WithContext(ctx)
}

func isAdminAuthorized(r *http.Request) bool {
	if r == nil {
		return false
	}
	authorized, _ := r.Context().Value(adminAuthContextKey{}).(bool)
	return authorized
}

func currentAdminAuthorizedSession(r *http.Request) *authorizedSession {
	if r == nil {
		return nil
	}
	auth, _ := r.Context().Value(adminAuthorizedSessionContextKey{}).(*authorizedSession)
	return auth
}
