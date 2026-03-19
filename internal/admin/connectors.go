package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	storepkg "github.com/Jopoleon/telega-bot-fedor/internal/store"
)

var (
	errCreateConnectorRequired  = errors.New("create_connector_required")
	errCreateConnectorPrice     = errors.New("create_connector_price")
	errCreateConnectorPeriod    = errors.New("create_connector_period")
	errCreateConnectorChatOrURL = errors.New("create_connector_chat_or_url_required")
)

// connectorsPage handles list/create connector operations.
func (h *Handler) connectorsPage(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)

	switch r.Method {
	case http.MethodGet:
		h.renderConnectorsPage(r.Context(), w, r, lang, "")
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			h.renderConnectorsPage(r.Context(), w, r, lang, t(lang, "connectors.bad_form"))
			return
		}
		if !h.verifyCSRF(r) {
			w.WriteHeader(http.StatusForbidden)
			h.renderConnectorsPage(r.Context(), w, r, lang, t(lang, "csrf.invalid"))
			return
		}
		if err := h.createConnector(r.Context(), r); err != nil {
			h.renderConnectorsPage(r.Context(), w, r, lang, h.localizeCreateConnectorError(lang, err))
			return
		}
		h.renderConnectorsPage(r.Context(), w, r, lang, t(lang, "connectors.created"))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// toggleConnector switches connector active state without deleting history.
func (h *Handler) toggleConnector(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad form"))
		return
	}
	if !h.verifyCSRF(r) {
		w.WriteHeader(http.StatusForbidden)
		h.renderConnectorsPage(r.Context(), w, r, lang, t(lang, "csrf.invalid"))
		return
	}
	idRaw := strings.TrimSpace(r.FormValue("id"))
	active := r.FormValue("active") == "true"
	if idRaw == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("id is required"))
		return
	}
	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil || id <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid id"))
		return
	}
	if err := h.store.SetConnectorActive(r.Context(), id, active); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	h.renderConnectorsPage(r.Context(), w, r, lang, t(lang, "connectors.updated"))
}

// deleteConnector hard-deletes connector if it has no dependent history.
func (h *Handler) deleteConnector(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderConnectorsPage(r.Context(), w, r, lang, t(lang, "connectors.bad_form"))
		return
	}
	if !h.verifyCSRF(r) {
		w.WriteHeader(http.StatusForbidden)
		h.renderConnectorsPage(r.Context(), w, r, lang, t(lang, "csrf.invalid"))
		return
	}

	id, err := parseConnectorID(r.FormValue("id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		h.renderConnectorsPage(r.Context(), w, r, lang, t(lang, "connectors.invalid_id"))
		return
	}

	if err := h.store.DeleteConnector(r.Context(), id); err != nil {
		h.renderConnectorsPage(r.Context(), w, r, lang, h.localizeDeleteConnectorError(lang, err))
		return
	}

	h.renderConnectorsPage(r.Context(), w, r, lang, t(lang, "connectors.deleted"))
}

// createConnector parses HTML form and persists connector entity.
func (h *Handler) createConnector(ctx context.Context, r *http.Request) error {
	startPayload := strings.TrimSpace(r.FormValue("start_payload"))
	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))
	chatID := strings.TrimSpace(r.FormValue("chat_id"))
	priceRaw := strings.TrimSpace(r.FormValue("price_rub"))
	periodRaw := strings.TrimSpace(r.FormValue("period_days"))
	offerURL := strings.TrimSpace(r.FormValue("offer_url"))
	privacyURL := strings.TrimSpace(r.FormValue("privacy_url"))
	channelURL := strings.TrimSpace(r.FormValue("channel_url"))

	if startPayload == "" {
		startPayload = "in-" + generateToken(8)
	}
	if periodRaw == "" {
		periodRaw = "30"
	}

	if name == "" || priceRaw == "" {
		return errCreateConnectorRequired
	}
	if chatID == "" && channelURL == "" {
		return errCreateConnectorChatOrURL
	}
	// Keep chat ID in unsigned form to stay consistent with current admin input convention.
	chatID = strings.TrimPrefix(chatID, "-")

	price, err := strconv.ParseInt(priceRaw, 10, 64)
	if err != nil || price <= 0 {
		return errCreateConnectorPrice
	}
	periodDays, err := strconv.Atoi(periodRaw)
	if err != nil || periodDays <= 0 {
		return errCreateConnectorPeriod
	}

	connector := domain.Connector{
		StartPayload: startPayload,
		Name:         name,
		Description:  description,
		ChatID:       chatID,
		ChannelURL:   channelURL,
		PriceRUB:     price,
		PeriodDays:   periodDays,
		OfferURL:     offerURL,
		PrivacyURL:   privacyURL,
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
	}

	if err := h.store.CreateConnector(ctx, connector); err != nil {
		return err
	}

	return nil
}

// renderConnectorsPage maps domain models to view models and renders template.
func (h *Handler) renderConnectorsPage(ctx context.Context, w http.ResponseWriter, r *http.Request, lang, notice string) {
	connectors, _ := h.store.ListConnectors(ctx)

	botUsername := h.botUsername
	if botUsername == "" {
		botUsername = "<bot_username>"
	}

	rows := make([]connectorView, 0, len(connectors))
	for _, c := range connectors {
		toggleTo := !c.IsActive
		toggleLabel := t(lang, "connectors.table.enable")
		if c.IsActive {
			toggleLabel = t(lang, "connectors.table.disable")
		}
		activeLabel, activeClass := connectorActiveBadge(lang, c.IsActive)
		rows = append(rows, connectorView{
			ID:           c.ID,
			StartPayload: c.StartPayload,
			Name:         c.Name,
			ChatID:       c.ChatID,
			ChannelURL:   c.ChannelURL,
			PriceRUB:     c.PriceRUB,
			PeriodDays:   c.PeriodDays,
			OfferURL:     c.OfferURL,
			PrivacyURL:   c.PrivacyURL,
			BotLink:      "https://t.me/" + botUsername + "?start=" + c.StartPayload,
			IsActive:     c.IsActive,
			ActiveLabel:  activeLabel,
			ActiveClass:  activeClass,
			ToggleTo:     toggleTo,
			ToggleLabel:  toggleLabel,
		})
	}

	h.renderer.render(w, "connectors.html", connectorsPageData{
		basePageData: basePageData{
			Lang:       lang,
			I18N:       dictForLang(lang),
			CSRFToken:  h.ensureCSRFToken(w, r),
			TopbarPath: "/admin/connectors",
			ActiveNav:  "connectors",
		},
		Notice:          notice,
		RequiredMessage: t(lang, "connectors.required"),
		Connectors:      rows,
	})
}

func (h *Handler) localizeCreateConnectorError(lang string, err error) string {
	switch {
	case errors.Is(err, errCreateConnectorRequired):
		return t(lang, "connectors.required")
	case errors.Is(err, errCreateConnectorPrice):
		return t(lang, "connector.validation.price")
	case errors.Is(err, errCreateConnectorPeriod):
		return t(lang, "connector.validation.period")
	case errors.Is(err, errCreateConnectorChatOrURL):
		return t(lang, "connector.validation.chat_or_url")
	default:
		return err.Error()
	}
}

func (h *Handler) localizeDeleteConnectorError(lang string, err error) string {
	switch {
	case errors.Is(err, storepkg.ErrConnectorNotFound):
		return t(lang, "connectors.not_found")
	case errors.Is(err, storepkg.ErrConnectorInUse):
		return t(lang, "connectors.delete_in_use")
	default:
		return err.Error()
	}
}

func parseConnectorID(raw string) (int64, error) {
	idRaw := strings.TrimSpace(raw)
	if idRaw == "" {
		return 0, errors.New("empty id")
	}
	id, err := strconv.ParseInt(idRaw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

// generateToken creates random hex token for IDs/payloads in admin form defaults.
func generateToken(size int) string {
	if size <= 0 {
		size = 8
	}
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return hex.EncodeToString(raw)
}
