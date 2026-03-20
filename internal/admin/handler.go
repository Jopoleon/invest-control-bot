package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
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

func (h *Handler) logAdminAudit(r *http.Request, action, details string) {
	_ = h.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
		Action:    action,
		Details:   details,
		CreatedAt: time.Now().UTC(),
	})
}

// Register mounts admin routes into provided mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("/admin/assets/", http.StripPrefix("/admin/assets/", staticHandler()))
	mux.HandleFunc("/admin/login", h.loginPage)

	protected := http.NewServeMux()
	protected.HandleFunc("/admin/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/connectors", http.StatusFound)
	})
	protected.HandleFunc("/admin/logout", h.logout)
	protected.HandleFunc("/admin/connectors", h.connectorsPage)
	protected.HandleFunc("/admin/connectors/toggle", h.toggleConnector)
	protected.HandleFunc("/admin/connectors/delete", h.deleteConnector)
	protected.HandleFunc("/admin/connectors/export.csv", h.exportConnectorsCSV)
	protected.HandleFunc("/admin/legal-documents", h.legalDocumentsPage)
	protected.HandleFunc("/admin/legal-documents/toggle", h.toggleLegalDocument)
	protected.HandleFunc("/admin/legal-documents/delete", h.deleteLegalDocument)
	protected.HandleFunc("/admin/legal-documents/export.csv", h.exportLegalDocumentsCSV)
	protected.HandleFunc("/admin/users", h.usersPage)
	protected.HandleFunc("/admin/users/view", h.userDetailPage)
	protected.HandleFunc("/admin/users/export.csv", h.exportUsersCSV)
	protected.HandleFunc("/admin/users/message", h.sendUserMessage)
	protected.HandleFunc("/admin/users/send-payment-link", h.sendUserPaymentLink)
	protected.HandleFunc("/admin/subscriptions/revoke", h.revokeSubscription)
	protected.HandleFunc("/admin/billing", h.billingPage)
	protected.HandleFunc("/admin/billing/payments/export.csv", h.exportPaymentsCSV)
	protected.HandleFunc("/admin/billing/subscriptions/export.csv", h.exportSubscriptionsCSV)
	protected.HandleFunc("/admin/churn", h.churnPage)
	protected.HandleFunc("/admin/churn/export.csv", h.exportChurnCSV)
	protected.HandleFunc("/admin/events", h.eventsPage)
	protected.HandleFunc("/admin/events/export.csv", h.exportEventsCSV)
	protected.HandleFunc("/admin/help", h.helpPage)

	mux.Handle("/admin/", h.withAdminSession(protected))
}
