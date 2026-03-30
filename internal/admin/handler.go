package admin

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/recurringlink"
	"github.com/Jopoleon/invest-control-bot/internal/store"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
	"github.com/go-chi/chi/v5"
)

// Handler serves admin HTTP pages and operations for connector management.
type Handler struct {
	store           store.Store
	adminToken      string
	botUsername     string
	maxBotUsername  string
	publicBaseURL   string
	encryptionKey   string
	tg              *telegram.Client
	renderer        *renderer
	retriggerRebill func(ctx context.Context, subscriptionID int64) (RebillResult, error)

	loginRateLimiter *loginRateLimiter
}

type RebillResult struct {
	InvoiceID string
	Existing  bool
}

// NewHandler creates admin handler and preloads HTML templates.
func NewHandler(st store.Store, adminToken, botUsername, maxBotUsername, publicBaseURL, encryptionKey string, tg *telegram.Client, rebillTrigger func(context.Context, int64) (RebillResult, error)) *Handler {
	r, err := newRenderer()
	if err != nil {
		panic(err)
	}
	return &Handler{
		store:           st,
		adminToken:      adminToken,
		botUsername:     strings.TrimPrefix(strings.TrimSpace(botUsername), "@"),
		maxBotUsername:  strings.TrimPrefix(strings.TrimSpace(maxBotUsername), "@"),
		publicBaseURL:   strings.TrimRight(strings.TrimSpace(publicBaseURL), "/"),
		encryptionKey:   strings.TrimSpace(encryptionKey),
		tg:              tg,
		renderer:        r,
		retriggerRebill: rebillTrigger,

		loginRateLimiter: newLoginRateLimiter(),
	}
}

func (h *Handler) buildAutopayCancelURL(telegramID int64) string {
	if telegramID <= 0 || h.publicBaseURL == "" || h.encryptionKey == "" {
		return ""
	}
	token, err := recurringlink.BuildCancelToken(h.encryptionKey, telegramID, time.Now().UTC().Add(180*24*time.Hour))
	if err != nil {
		return ""
	}
	return h.publicBaseURL + "/unsubscribe/" + token
}

func (h *Handler) logAdminAudit(r *http.Request, action, details string) {
	_ = h.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
		ActorType:    domain.AuditActorTypeAdmin,
		ActorSubject: "admin_panel",
		Action:       action,
		Details:      details,
		CreatedAt:    time.Now().UTC(),
	})
}

func (h *Handler) logAdminTargetAudit(r *http.Request, user domain.User, connectorID int64, action, details string) {
	event := domain.AuditEvent{
		ActorType:    domain.AuditActorTypeAdmin,
		ActorSubject: "admin_panel",
		TargetUserID: user.ID,
		ConnectorID:  connectorID,
		Action:       action,
		Details:      details,
		CreatedAt:    time.Now().UTC(),
	}
	if telegramID, _, found, err := h.resolveTelegramIdentity(r.Context(), user.ID); err == nil && found {
		event.TargetMessengerKind = domain.MessengerKindTelegram
		event.TargetMessengerUserID = strconv.FormatInt(telegramID, 10)
	}
	_ = h.store.SaveAuditEvent(r.Context(), event)
}

// Register mounts admin routes into the shared application router.
func (h *Handler) Register(router chi.Router) {
	router.Handle("/admin/assets/*", http.StripPrefix("/admin/assets/", staticHandler()))
	router.HandleFunc("/admin/login", h.loginPage)
	router.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/connectors", http.StatusFound)
	})

	router.Route("/admin", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return h.withAdminSession(next)
		})
		r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/admin/connectors", http.StatusFound)
		})
		r.HandleFunc("/logout", h.logout)
		r.HandleFunc("/connectors", h.connectorsPage)
		r.HandleFunc("/connectors/toggle", h.toggleConnector)
		r.HandleFunc("/connectors/delete", h.deleteConnector)
		r.HandleFunc("/connectors/export.csv", h.exportConnectorsCSV)
		r.HandleFunc("/legal-documents", h.legalDocumentsPage)
		r.HandleFunc("/legal-documents/toggle", h.toggleLegalDocument)
		r.HandleFunc("/legal-documents/delete", h.deleteLegalDocument)
		r.HandleFunc("/legal-documents/export.csv", h.exportLegalDocumentsCSV)
		r.HandleFunc("/users", h.usersPage)
		r.HandleFunc("/users/view", h.userDetailPage)
		r.HandleFunc("/users/export.csv", h.exportUsersCSV)
		r.HandleFunc("/users/message", h.sendUserMessage)
		r.HandleFunc("/users/send-payment-link", h.sendUserPaymentLink)
		r.HandleFunc("/subscriptions/revoke", h.revokeSubscription)
		r.HandleFunc("/subscriptions/rebill", h.triggerSubscriptionRebill)
		r.HandleFunc("/billing", h.billingPage)
		r.HandleFunc("/billing/payments/export.csv", h.exportPaymentsCSV)
		r.HandleFunc("/billing/subscriptions/export.csv", h.exportSubscriptionsCSV)
		r.HandleFunc("/churn", h.churnPage)
		r.HandleFunc("/churn/export.csv", h.exportChurnCSV)
		r.HandleFunc("/events", h.eventsPage)
		r.HandleFunc("/events/export.csv", h.exportEventsCSV)
		r.HandleFunc("/sessions", h.sessionsPage)
		r.HandleFunc("/sessions/revoke", h.revokeAdminSession)
		r.HandleFunc("/help", h.helpPage)
	})
}
