package admin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	storepkg "github.com/Jopoleon/invest-control-bot/internal/store"
)

var (
	errCreateConnectorRequired   = errors.New("create_connector_required")
	errCreateConnectorPrice      = errors.New("create_connector_price")
	errCreateConnectorPeriodMode = errors.New("create_connector_period_mode")
	errCreateConnectorDuration   = errors.New("create_connector_duration")
	errCreateConnectorMonths     = errors.New("create_connector_months")
	errCreateConnectorDeadline   = errors.New("create_connector_deadline")
	errCreateConnectorChatOrURL  = errors.New("create_connector_chat_or_url_required")
)

// connectorsPage handles list/create connector operations.
func (h *Handler) connectorsPage(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)

	switch r.Method {
	case http.MethodGet:
		h.renderConnectorsPage(r.Context(), w, r, lang, strings.TrimSpace(r.URL.Query().Get("notice")))
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
		h.redirectConnectors(w, r, lang, t(lang, "connectors.created"))
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

	h.redirectConnectors(w, r, lang, t(lang, "connectors.updated"))
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

	h.redirectConnectors(w, r, lang, t(lang, "connectors.deleted"))
}

// createConnector parses HTML form and persists connector entity.
func (h *Handler) createConnector(ctx context.Context, r *http.Request) error {
	startPayload := strings.TrimSpace(r.FormValue("start_payload"))
	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))
	chatID := strings.TrimSpace(r.FormValue("chat_id"))
	maxChannelURL := strings.TrimSpace(r.FormValue("max_channel_url"))
	priceRaw := strings.TrimSpace(r.FormValue("price_rub"))
	periodModeRaw := strings.TrimSpace(r.FormValue("period_mode"))
	durationRaw := strings.TrimSpace(r.FormValue("period_value"))
	monthsRaw := strings.TrimSpace(r.FormValue("period_months"))
	fixedEndsAtRaw := strings.TrimSpace(r.FormValue("fixed_ends_at"))
	offerURL := strings.TrimSpace(r.FormValue("offer_url"))
	privacyURL := strings.TrimSpace(r.FormValue("privacy_url"))
	channelURL := strings.TrimSpace(r.FormValue("channel_url"))

	if startPayload == "" {
		startPayload = "in-" + generateToken(8)
	}

	if name == "" || priceRaw == "" {
		return errCreateConnectorRequired
	}
	if chatID == "" && channelURL == "" && maxChannelURL == "" {
		return errCreateConnectorChatOrURL
	}
	// Keep chat ID in unsigned form to stay consistent with current admin input convention.
	chatID = strings.TrimPrefix(chatID, "-")

	price, err := strconv.ParseInt(priceRaw, 10, 64)
	if err != nil || price <= 0 {
		return errCreateConnectorPrice
	}
	periodMode, periodSeconds, periodMonths, fixedEndsAt, err := parseConnectorPeriodModel(periodModeRaw, durationRaw, monthsRaw, fixedEndsAtRaw, time.Now().UTC())
	if err != nil {
		return err
	}

	connector := domain.Connector{
		StartPayload:  startPayload,
		Name:          name,
		Description:   description,
		ChatID:        chatID,
		ChannelURL:    channelURL,
		MAXChannelURL: maxChannelURL,
		PriceRUB:      price,
		PeriodMode:    periodMode,
		PeriodSeconds: periodSeconds,
		PeriodMonths:  periodMonths,
		FixedEndsAt:   fixedEndsAt,
		OfferURL:      offerURL,
		PrivacyURL:    privacyURL,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
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
	maxBotUsername := h.maxBotUsername
	if maxBotUsername == "" {
		maxBotUsername = "<max_bot_username>"
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
			ID:              c.ID,
			StartPayload:    c.StartPayload,
			Name:            c.Name,
			ChatID:          c.ChatID,
			TelegramURL:     c.TelegramAccessURL(),
			MAXChannelURL:   c.MAXChannelURL,
			PriceRUB:        c.PriceRUB,
			PeriodLabel:     adminConnectorPeriodLabel(c),
			OfferURL:        c.OfferURL,
			PrivacyURL:      c.PrivacyURL,
			TelegramBotLink: buildAdminBotStartURL(botUsername, c.StartPayload),
			MAXBotLink:      buildAdminMAXStartURL(maxBotUsername, c.StartPayload),
			MAXStartCommand: buildAdminStartCommand(c.StartPayload),
			IsActive:        c.IsActive,
			ActiveLabel:     activeLabel,
			ActiveClass:     activeClass,
			ToggleTo:        toggleTo,
			ToggleLabel:     toggleLabel,
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
		Notice:              notice,
		RequiredMessage:     t(lang, "connectors.required"),
		ExportURL:           buildExportURL("/admin/connectors/export.csv", r.URL.Query(), lang),
		TelegramBotUsername: botUsername,
		MAXBotUsername:      maxBotUsername,
		Connectors:          rows,
	})
}

func (h *Handler) redirectConnectors(w http.ResponseWriter, r *http.Request, lang, notice string) {
	query := url.Values{}
	query.Set("lang", lang)
	if strings.TrimSpace(notice) != "" {
		query.Set("notice", notice)
	}
	http.Redirect(w, r, "/admin/connectors?"+query.Encode(), http.StatusSeeOther)
}

func (h *Handler) localizeCreateConnectorError(lang string, err error) string {
	switch {
	case errors.Is(err, errCreateConnectorRequired):
		return t(lang, "connectors.required")
	case errors.Is(err, errCreateConnectorPrice):
		return t(lang, "connector.validation.price")
	case errors.Is(err, errCreateConnectorPeriodMode):
		return t(lang, "connector.validation.period_mode")
	case errors.Is(err, errCreateConnectorDuration):
		return t(lang, "connector.validation.period_duration")
	case errors.Is(err, errCreateConnectorMonths):
		return t(lang, "connector.validation.period_months")
	case errors.Is(err, errCreateConnectorDeadline):
		return t(lang, "connector.validation.period_deadline")
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

func parseConnectorPeriodModel(periodModeRaw, durationRaw, monthsRaw, fixedEndsAtRaw string, now time.Time) (domain.ConnectorPeriodMode, int64, int, *time.Time, error) {
	periodMode := domain.ConnectorPeriodMode(strings.TrimSpace(periodModeRaw))
	if periodMode == "" {
		periodMode = domain.ConnectorPeriodModeDuration
	}

	switch periodMode {
	case domain.ConnectorPeriodModeDuration:
		if durationRaw == "" {
			durationRaw = "30d"
		}
		seconds, err := parseConnectorDuration(durationRaw)
		if err != nil {
			return "", 0, 0, nil, errCreateConnectorDuration
		}
		return periodMode, seconds, 0, nil, nil
	case domain.ConnectorPeriodModeCalendarMonths:
		months, err := strconv.Atoi(strings.TrimSpace(monthsRaw))
		if err != nil || months <= 0 {
			return "", 0, 0, nil, errCreateConnectorMonths
		}
		return periodMode, 0, months, nil, nil
	case domain.ConnectorPeriodModeFixedDeadline:
		fixedEndsAt, err := parseConnectorFixedDeadline(fixedEndsAtRaw)
		if err != nil || !fixedEndsAt.After(now) {
			return "", 0, 0, nil, errCreateConnectorDeadline
		}
		return periodMode, 0, 0, &fixedEndsAt, nil
	default:
		return "", 0, 0, nil, errCreateConnectorPeriodMode
	}
}

func parseConnectorDuration(raw string) (int64, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0, errors.New("empty duration")
	}
	if strings.HasSuffix(raw, "d") {
		daysRaw := strings.TrimSpace(strings.TrimSuffix(raw, "d"))
		days, err := strconv.ParseInt(daysRaw, 10, 64)
		if err != nil || days <= 0 {
			return 0, errors.New("invalid day duration")
		}
		return days * 24 * 60 * 60, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	seconds := int64(d / time.Second)
	if seconds <= 0 {
		return 0, errors.New("duration must be at least one second")
	}
	return seconds, nil
}

func parseConnectorFixedDeadline(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("empty deadline")
	}
	for _, layout := range []string{"2006-01-02T15:04", time.RFC3339} {
		if ts, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return ts.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid deadline %q", raw)
}

func adminConnectorPeriodLabel(c domain.Connector) string {
	if fixedEndsAt, ok := c.FixedDeadline(); ok {
		return "до " + fixedEndsAt.In(time.Local).Format("02.01.2006 15:04")
	}
	if months, ok := c.CalendarMonthsPeriod(); ok {
		return strconv.Itoa(months) + " мес."
	}
	if duration, ok := c.DurationPeriod(); ok {
		return formatAdminDurationLabel(duration)
	}
	return "30 дн."
}

func formatAdminDurationLabel(duration time.Duration) string {
	seconds := int64(duration / time.Second)
	switch {
	case seconds%(24*60*60) == 0:
		return strconv.FormatInt(seconds/(24*60*60), 10) + " дн."
	case seconds%(60*60) == 0:
		return strconv.FormatInt(seconds/(60*60), 10) + " ч."
	case seconds%60 == 0:
		return strconv.FormatInt(seconds/60, 10) + " мин."
	default:
		return strconv.FormatInt(seconds, 10) + " сек."
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
