package admin

import (
	"net/http"
	"strings"

	"github.com/Jopoleon/telega-bot-fedor/internal/store"
)

// Handler serves admin HTTP pages and operations for connector management.
type Handler struct {
	store       store.Store
	adminToken  string
	botUsername string
	renderer    *renderer
}

// NewHandler creates admin handler and preloads HTML templates.
func NewHandler(st store.Store, adminToken, botUsername string) *Handler {
	r, err := newRenderer()
	if err != nil {
		panic(err)
	}
	return &Handler{
		store:       st,
		adminToken:  adminToken,
		botUsername: strings.TrimPrefix(strings.TrimSpace(botUsername), "@"),
		renderer:    r,
	}
}

// Register mounts admin routes into provided mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("/admin/assets/", http.StripPrefix("/admin/assets/", staticHandler()))
	mux.HandleFunc("/admin/connectors", h.connectorsPage)
	mux.HandleFunc("/admin/connectors/toggle", h.toggleConnector)
	mux.HandleFunc("/admin/billing", h.billingPage)
	mux.HandleFunc("/admin/events", h.eventsPage)
	mux.HandleFunc("/admin/help", h.helpPage)
}
