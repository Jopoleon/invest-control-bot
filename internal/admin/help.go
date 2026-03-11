package admin

import "net/http"

// helpPage renders quick usage reference for admin operators.
func (h *Handler) helpPage(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		h.unauthorized(w)
		return
	}
	h.persistTokenCookie(w, r)
	lang := h.resolveLang(w, r)
	h.renderer.render(w, "help.html", helpPageData{
		basePageData: basePageData{
			Lang: lang,
			I18N: dictForLang(lang),
		},
		BotUsername: h.botUsername,
	})
}
