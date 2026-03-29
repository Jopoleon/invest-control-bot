package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (h *Handler) usersPage(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)

	data := usersPageData{
		basePageData: basePageData{
			Lang:       lang,
			I18N:       dictForLang(lang),
			CSRFToken:  h.ensureCSRFToken(w, r),
			TopbarPath: "/admin/users",
			ActiveNav:  "users",
		},
		TelegramID: strings.TrimSpace(r.URL.Query().Get("telegram_id")),
		Search:     strings.TrimSpace(r.URL.Query().Get("search")),
		ExportURL:  buildExportURL("/admin/users/export.csv", r.URL.Query(), lang),
	}

	query := domain.UserListQuery{Limit: 300, Search: data.Search}
	userID, err := h.resolveFilterUserID(r.Context(), r.URL.Query().Get("user_id"), data.TelegramID)
	if err != nil {
		data.Notice = t(lang, "users.load_error")
		h.renderer.render(w, "users.html", data)
		return
	}
	if userID > 0 {
		query.UserID = userID
	}

	users, err := h.store.ListUsers(r.Context(), query)
	if err != nil {
		data.Notice = t(lang, "users.load_error")
		h.renderer.render(w, "users.html", data)
		return
	}

	data.Users = make([]userView, 0, len(users))
	for _, user := range users {
		autoPayLabel, autoPayClass := autoPayBadge(lang, user.AutoPayEnabled, user.HasAutoPaySettings)
		data.Users = append(data.Users, userView{
			UserID:           user.UserID,
			TelegramID:       user.TelegramID,
			TelegramUsername: user.TelegramUsername,
			FullName:         user.FullName,
			Phone:            user.Phone,
			Email:            user.Email,
			AutoPay:          autoPayLabel,
			AutoPayClass:     autoPayClass,
			UpdatedAt:        user.UpdatedAt.In(time.Local).Format("2006-01-02 15:04:05"),
			DetailURL:        buildUserDetailURL(lang, user.UserID, user.TelegramID),
		})
	}

	h.renderer.render(w, "users.html", data)
}
