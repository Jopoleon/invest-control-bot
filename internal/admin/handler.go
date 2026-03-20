package admin

import (
	"net/http"
	"strings"

	"github.com/Jopoleon/telega-bot-fedor/internal/store"
	"github.com/Jopoleon/telega-bot-fedor/internal/telegram"
)

// Handler serves admin HTTP pages and operations for connector management.
type Handler struct {
	store       store.Store
	adminToken  string
	botUsername string
	tg          *telegram.Client
	renderer    *renderer

	loginRateLimiter *loginRateLimiter
}

// NewHandler creates admin handler and preloads HTML templates.
func NewHandler(st store.Store, adminToken, botUsername string, tg *telegram.Client) *Handler {
	r, err := newRenderer()
	if err != nil {
		panic(err)
	}
	return &Handler{
		store:       st,
		adminToken:  adminToken,
		botUsername: strings.TrimPrefix(strings.TrimSpace(botUsername), "@"),
		tg:          tg,
		renderer:    r,

		loginRateLimiter: newLoginRateLimiter(),
	}
}

// Register mounts admin routes into provided mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("/admin/assets/", http.StripPrefix("/admin/assets/", staticHandler()))
	mux.HandleFunc("/admin/login", h.loginPage)
	mux.HandleFunc("/admin/logout", h.logout)
	mux.HandleFunc("/admin/connectors", h.connectorsPage)
	mux.HandleFunc("/admin/connectors/toggle", h.toggleConnector)
	mux.HandleFunc("/admin/connectors/delete", h.deleteConnector)
	mux.HandleFunc("/admin/connectors/export.csv", h.exportConnectorsCSV)
	mux.HandleFunc("/admin/users", h.usersPage)
	mux.HandleFunc("/admin/users/view", h.userDetailPage)
	mux.HandleFunc("/admin/users/export.csv", h.exportUsersCSV)
	mux.HandleFunc("/admin/users/message", h.sendUserMessage)
	mux.HandleFunc("/admin/users/send-payment-link", h.sendUserPaymentLink)
	mux.HandleFunc("/admin/subscriptions/revoke", h.revokeSubscription)
	mux.HandleFunc("/admin/billing", h.billingPage)
	mux.HandleFunc("/admin/billing/payments/export.csv", h.exportPaymentsCSV)
	mux.HandleFunc("/admin/billing/subscriptions/export.csv", h.exportSubscriptionsCSV)
	mux.HandleFunc("/admin/churn", h.churnPage)
	mux.HandleFunc("/admin/churn/export.csv", h.exportChurnCSV)
	mux.HandleFunc("/admin/events", h.eventsPage)
	mux.HandleFunc("/admin/events/export.csv", h.exportEventsCSV)
	mux.HandleFunc("/admin/help", h.helpPage)
}
