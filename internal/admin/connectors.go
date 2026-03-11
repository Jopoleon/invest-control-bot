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
)

var (
	errCreateConnectorRequired = errors.New("create_connector_required")
	errCreateConnectorPrice    = errors.New("create_connector_price")
	errCreateConnectorPeriod   = errors.New("create_connector_period")
)

// connectorsPage handles list/create connector operations.
func (h *Handler) connectorsPage(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		h.unauthorized(w)
		return
	}
	h.persistTokenCookie(w, r)
	lang := h.resolveLang(w, r)

	switch r.Method {
	case http.MethodGet:
		h.renderConnectorsPage(r.Context(), w, lang, "")
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			h.renderConnectorsPage(r.Context(), w, lang, t(lang, "connectors.bad_form"))
			return
		}
		if err := h.createConnector(r.Context(), r); err != nil {
			h.renderConnectorsPage(r.Context(), w, lang, h.localizeCreateConnectorError(lang, err))
			return
		}
		h.renderConnectorsPage(r.Context(), w, lang, t(lang, "connectors.created"))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// toggleConnector switches connector active state without deleting history.
func (h *Handler) toggleConnector(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		h.unauthorized(w)
		return
	}
	h.persistTokenCookie(w, r)
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
	id := strings.TrimSpace(r.FormValue("id"))
	active := r.FormValue("active") == "true"
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("id is required"))
		return
	}
	if err := h.store.SetConnectorActive(r.Context(), id, active); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(err.Error()))
		return
	}

	h.renderConnectorsPage(r.Context(), w, lang, t(lang, "connectors.updated"))
}

// createConnector parses HTML form and persists connector entity.
func (h *Handler) createConnector(ctx context.Context, r *http.Request) error {
	id := strings.TrimSpace(r.FormValue("id"))
	startPayload := strings.TrimSpace(r.FormValue("start_payload"))
	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))
	chatID := strings.TrimSpace(r.FormValue("chat_id"))
	priceRaw := strings.TrimSpace(r.FormValue("price_rub"))
	periodRaw := strings.TrimSpace(r.FormValue("period_days"))
	offerURL := strings.TrimSpace(r.FormValue("offer_url"))
	privacyURL := strings.TrimSpace(r.FormValue("privacy_url"))

	if id == "" {
		id = generateToken(8)
	}
	if startPayload == "" {
		startPayload = "in-" + generateToken(8)
	}
	if periodRaw == "" {
		periodRaw = "30"
	}

	if name == "" || chatID == "" || priceRaw == "" {
		return errCreateConnectorRequired
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
		ID:           id,
		StartPayload: startPayload,
		Name:         name,
		Description:  description,
		ChatID:       chatID,
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
func (h *Handler) renderConnectorsPage(ctx context.Context, w http.ResponseWriter, lang, notice string) {
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
		rows = append(rows, connectorView{
			ID:           c.ID,
			StartPayload: c.StartPayload,
			Name:         c.Name,
			ChatID:       c.ChatID,
			PriceRUB:     c.PriceRUB,
			PeriodDays:   c.PeriodDays,
			OfferURL:     c.OfferURL,
			PrivacyURL:   c.PrivacyURL,
			BotLink:      "https://t.me/" + botUsername + "?start=" + c.StartPayload,
			IsActive:     c.IsActive,
			ToggleTo:     toggleTo,
			ToggleLabel:  toggleLabel,
		})
	}

	h.renderer.render(w, "connectors.html", connectorsPageData{
		basePageData: basePageData{
			Lang: lang,
			I18N: dictForLang(lang),
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
	default:
		return err.Error()
	}
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
