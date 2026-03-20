package admin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
)

// loginPage renders login form and issues admin cookie on successful token validation.
func (h *Handler) loginPage(w http.ResponseWriter, r *http.Request) {
	lang := h.resolveLang(w, r)
	next := strings.TrimSpace(r.URL.Query().Get("next"))
	if next == "" || !strings.HasPrefix(next, "/admin/") {
		next = "/admin/connectors"
	}

	if h.adminToken == "" {
		http.Redirect(w, r, next, http.StatusFound)
		return
	}
	if h.authorized(w, r) {
		http.Redirect(w, r, next, http.StatusFound)
		return
	}

	data := loginPageData{
		basePageData: basePageData{
			Lang:       lang,
			I18N:       dictForLang(lang),
			CSRFToken:  h.ensureCSRFToken(w, r),
			TopbarPath: "/admin/login",
		},
		Next: next,
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			data.Notice = t(lang, "login.bad_form")
			h.renderer.render(w, "login.html", data)
			return
		}
		if !h.loginRateLimiter.Allow(r) {
			w.WriteHeader(http.StatusTooManyRequests)
			data.Notice = t(lang, "login.rate_limited")
			h.renderer.render(w, "login.html", data)
			return
		}
		if !h.verifyCSRF(r) {
			w.WriteHeader(http.StatusForbidden)
			data.Notice = t(lang, "csrf.invalid")
			h.renderer.render(w, "login.html", data)
			return
		}
		token := strings.TrimSpace(r.FormValue("token"))
		nextForm := strings.TrimSpace(r.FormValue("next"))
		if strings.HasPrefix(nextForm, "/admin/") {
			data.Next = nextForm
		}
		if token != h.adminToken {
			h.loginRateLimiter.RegisterFailure(r)
			h.logAdminAudit(r, domain.AuditActionAdminLoginFailed, fmt.Sprintf("ip=%s", clientIP(r)))
			data.Notice = t(lang, "login.bad_token")
			h.renderer.render(w, "login.html", data)
			return
		}
		h.loginRateLimiter.RegisterSuccess(r)
		if err := h.createAdminSession(w, r); err != nil {
			data.Notice = err.Error()
			h.renderer.render(w, "login.html", data)
			return
		}
		h.logAdminAudit(r, domain.AuditActionAdminLoginSuccess, fmt.Sprintf("ip=%s", clientIP(r)))
		http.Redirect(w, r, data.Next, http.StatusFound)
		return
	}

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	h.renderer.render(w, "login.html", data)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	h.revokeCurrentAdminSession(w, r)
	h.logAdminAudit(r, domain.AuditActionAdminLogout, fmt.Sprintf("ip=%s", clientIP(r)))
	http.Redirect(w, r, "/admin/login", http.StatusFound)
}
