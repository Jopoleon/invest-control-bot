package admin

import (
	"net/http"
	"strconv"
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
		UserID:     strings.TrimSpace(r.URL.Query().Get("user_id")),
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
		// TODO: Replace this bounded N+1 lookup with a bulk account projection
		// when the admin user list store query becomes fully messenger-neutral.
		accounts, err := h.store.ListUserMessengerAccounts(r.Context(), user.UserID)
		if err != nil {
			data.Notice = t(lang, "users.load_error")
			h.renderer.render(w, "users.html", data)
			return
		}
		accountPresentation := buildMessengerAccountPresentation(lang, accounts)
		autoPayLabel, autoPayClass := autoPayBadge(lang, user.AutoPayEnabled, user.HasAutoPaySettings)
		data.Users = append(data.Users, userView{
			UserID:              user.UserID,
			DisplayName:         coalesceUserDisplayName(user.FullName, accountPresentation.DisplayName, user.UserID),
			PrimaryAccount:      accountPresentation.PrimaryAccount,
			LinkedAccounts:      accountPresentation.Accounts,
			HasTelegramIdentity: accountPresentation.HasTelegramIdentity,
			FullName:            user.FullName,
			Phone:               user.Phone,
			Email:               user.Email,
			AutoPay:             autoPayLabel,
			AutoPayClass:        autoPayClass,
			UpdatedAt:           user.UpdatedAt.In(time.Local).Format("2006-01-02 15:04:05"),
			DetailURL:           buildUserDetailURL(lang, user.UserID),
		})
	}

	h.renderer.render(w, "users.html", data)
}

func coalesceUserDisplayName(fullName, fallback string, userID int64) string {
	if strings.TrimSpace(fullName) != "" {
		return strings.TrimSpace(fullName)
	}
	if strings.TrimSpace(fallback) != "" {
		return strings.TrimSpace(fallback)
	}
	return "#" + strings.TrimSpace(strconv.FormatInt(userID, 10))
}
