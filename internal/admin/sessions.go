package admin

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
)

func (h *Handler) sessionsPage(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)

	rows, err := h.store.ListAdminSessions(r.Context(), 200)
	if err != nil {
		h.renderer.render(w, "sessions.html", adminSessionsPageData{
			basePageData: basePageData{
				Lang:       lang,
				I18N:       dictForLang(lang),
				CSRFToken:  h.ensureCSRFToken(w, r),
				TopbarPath: "/admin/sessions",
				ActiveNav:  "sessions",
			},
			Notice: t(lang, "sessions.load_error"),
		})
		return
	}

	current := currentAdminAuthorizedSession(r)
	now := time.Now().UTC()
	data := adminSessionsPageData{
		basePageData: basePageData{
			Lang:       lang,
			I18N:       dictForLang(lang),
			CSRFToken:  h.ensureCSRFToken(w, r),
			TopbarPath: "/admin/sessions",
			ActiveNav:  "sessions",
		},
		Notice: strings.TrimSpace(r.URL.Query().Get("notice")),
	}

	data.Sessions = make([]adminSessionView, 0, len(rows))
	for _, session := range rows {
		statusLabel, statusClass := adminSessionStatusBadge(lang, session, now)
		isCurrent := current != nil && current.session.ID == session.ID
		data.Sessions = append(data.Sessions, adminSessionView{
			ID:          session.ID,
			Subject:     session.Subject,
			IP:          session.IP,
			UserAgent:   session.UserAgent,
			CreatedAt:   session.CreatedAt.In(time.Local).Format("2006-01-02 15:04:05"),
			ExpiresAt:   session.ExpiresAt.In(time.Local).Format("2006-01-02 15:04:05"),
			LastSeenAt:  session.LastSeenAt.In(time.Local).Format("2006-01-02 15:04:05"),
			StatusLabel: statusLabel,
			StatusClass: statusClass,
			IsCurrent:   isCurrent,
			CanRevoke:   session.RevokedAt == nil && now.Before(session.ExpiresAt),
			RevokeURL:   "/admin/sessions/revoke?lang=" + lang,
		})
	}

	h.renderer.render(w, "sessions.html", data)
}

func (h *Handler) revokeAdminSession(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/sessions?lang="+lang+"&notice="+url.QueryEscape(t(lang, "sessions.load_error")), http.StatusFound)
		return
	}
	if !h.verifyCSRF(r) {
		w.WriteHeader(http.StatusForbidden)
		h.unauthorized(w)
		return
	}

	sessionID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("id")), 10, 64)
	if err != nil || sessionID <= 0 {
		http.Redirect(w, r, "/admin/sessions?lang="+lang+"&notice="+url.QueryEscape(t(lang, "sessions.invalid_id")), http.StatusFound)
		return
	}

	now := time.Now().UTC()
	if err := h.store.RevokeAdminSession(r.Context(), sessionID, now); err == nil {
		h.logAdminAudit(r, domain.AuditActionAdminSessionRevokedManual, "session_id="+strconv.FormatInt(sessionID, 10))
	}

	current := currentAdminAuthorizedSession(r)
	if current != nil && current.session.ID == sessionID {
		h.clearAdminSessionCookie(w, r)
		http.Redirect(w, r, "/admin/login", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/admin/sessions?lang="+lang+"&notice="+url.QueryEscape(t(lang, "sessions.revoked")), http.StatusFound)
}

func adminSessionStatusBadge(lang string, session domain.AdminSession, now time.Time) (string, string) {
	if session.RevokedAt != nil {
		return t(lang, "sessions.status.revoked"), "is-danger"
	}
	if now.After(session.ExpiresAt) {
		return t(lang, "sessions.status.expired"), "is-warning"
	}
	return t(lang, "sessions.status.active"), "is-success"
}
