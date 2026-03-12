package admin

import "net/http"

// helpPage renders quick usage reference for admin operators.
func (h *Handler) helpPage(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)
	h.renderer.render(w, "help.html", helpPageData{
		basePageData: basePageData{
			Lang:       lang,
			I18N:       dictForLang(lang),
			CSRFToken:  h.ensureCSRFToken(w, r),
			TopbarPath: "/admin/help",
			ActiveNav:  "guide",
		},
		BotUsername: h.botUsername,
	})
}
