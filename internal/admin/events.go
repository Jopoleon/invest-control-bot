package admin

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

// eventsPage renders audit event history with filtering, sorting and pagination.
func (h *Handler) eventsPage(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)

	data := eventsPageData{
		basePageData: basePageData{
			Lang:       lang,
			I18N:       dictForLang(lang),
			CSRFToken:  h.ensureCSRFToken(w, r),
			TopbarPath: "/admin/events",
			ActiveNav:  "events",
		},
		ExportURL: buildExportURL("/admin/events/export.csv", r.URL.Query(), lang),
		SortBy:    "created_at",
		SortDir:   "desc",
		Page:      1,
		PageSize:  50,
	}
	query := domain.AuditEventListQuery{
		SortBy:   data.SortBy,
		SortDesc: true,
		Page:     data.Page,
		PageSize: data.PageSize,
	}
	params := r.URL.Query()

	data.ActorType = strings.TrimSpace(params.Get("actor_type"))
	if data.ActorType == string(domain.AuditActorTypeAdmin) || data.ActorType == string(domain.AuditActorTypeUser) || data.ActorType == string(domain.AuditActorTypeApp) {
		query.ActorType = domain.AuditActorType(data.ActorType)
	}

	data.MessengerKind = strings.TrimSpace(params.Get("messenger_kind"))
	switch data.MessengerKind {
	case string(domain.MessengerKindTelegram), string(domain.MessengerKindMAX):
		query.TargetMessengerKind = domain.MessengerKind(data.MessengerKind)
	default:
		data.MessengerKind = ""
	}

	data.MessengerUserID = strings.TrimSpace(params.Get("messenger_user_id"))
	query.TargetMessengerUserID = data.MessengerUserID

	data.ConnectorID = strings.TrimSpace(params.Get("connector_id"))
	if data.ConnectorID != "" {
		if id, err := strconv.ParseInt(data.ConnectorID, 10, 64); err == nil && id > 0 {
			query.ConnectorID = id
		}
	}

	data.Action = strings.TrimSpace(params.Get("action"))
	query.Action = data.Action

	data.Search = strings.TrimSpace(params.Get("search"))
	query.Search = data.Search

	data.DateFrom = strings.TrimSpace(params.Get("date_from"))
	if data.DateFrom != "" {
		if from, ok := parseDateAtLocalStart(data.DateFrom); ok {
			query.CreatedFrom = &from
		}
	}

	data.DateTo = strings.TrimSpace(params.Get("date_to"))
	if data.DateTo != "" {
		if to, ok := parseDateAtLocalEndExclusive(data.DateTo); ok {
			query.CreatedToExclude = &to
		}
	}

	sortBy := strings.TrimSpace(params.Get("sort_by"))
	switch sortBy {
	case "actor_type", "target_messenger_user_id", "connector_id", "action", "created_at":
		data.SortBy = sortBy
	}
	query.SortBy = data.SortBy

	sortDir := strings.TrimSpace(strings.ToLower(params.Get("sort_dir")))
	if sortDir == "asc" {
		data.SortDir = "asc"
		query.SortDesc = false
	} else {
		data.SortDir = "desc"
		query.SortDesc = true
	}

	if pageSize, err := strconv.Atoi(strings.TrimSpace(params.Get("page_size"))); err == nil {
		switch {
		case pageSize > 200:
			data.PageSize = 200
		case pageSize >= 10:
			data.PageSize = pageSize
		}
	}
	query.PageSize = data.PageSize

	if page, err := strconv.Atoi(strings.TrimSpace(params.Get("page"))); err == nil && page > 0 {
		data.Page = page
	}
	query.Page = data.Page

	events, total, err := h.store.ListAuditEvents(r.Context(), query)
	if err != nil {
		h.renderer.render(w, "events.html", eventsPageData{
			basePageData: basePageData{
				Lang:       lang,
				I18N:       dictForLang(lang),
				CSRFToken:  h.ensureCSRFToken(w, r),
				TopbarPath: "/admin/events",
				ActiveNav:  "events",
			},
			Notice: t(lang, "events.load_error"),
		})
		return
	}
	connectorNames := h.loadConnectorNames(r.Context())

	rows := make([]auditEventView, 0, len(events))
	for _, event := range events {
		rows = append(rows, auditEventView{
			CreatedAt:             event.CreatedAt.In(time.Local).Format("2006-01-02 15:04:05"),
			ActorType:             string(event.ActorType),
			ActorUserID:           formatOptionalInt64(event.ActorUserID),
			ActorAccount:          buildMessengerAccountDisplay(messengerKindLabel(lang, event.ActorMessengerKind), event.ActorMessengerUserID, ""),
			ActorSubject:          event.ActorSubject,
			TargetUserID:          formatOptionalInt64(event.TargetUserID),
			TargetAccount:         buildMessengerAccountDisplay(messengerKindLabel(lang, event.TargetMessengerKind), event.TargetMessengerUserID, ""),
			TargetMessengerKind:   string(event.TargetMessengerKind),
			TargetMessengerUserID: event.TargetMessengerUserID,
			ConnectorID:           event.ConnectorID,
			Connector:             connectorDisplayName(connectorNames, event.ConnectorID),
			Action:                event.Action,
			Details:               event.Details,
		})
	}

	data.Rows = rows
	data.TotalItems = total
	data.TotalPages = (total + data.PageSize - 1) / data.PageSize
	if data.TotalPages == 0 {
		data.TotalPages = 1
	}
	if data.Page > data.TotalPages {
		data.Page = data.TotalPages
	}
	data.HasPrev = data.Page > 1
	data.HasNext = data.Page < data.TotalPages

	base := cloneURLValues(params)
	base.Set("lang", lang)
	base.Set("page_size", strconv.Itoa(data.PageSize))
	base.Set("sort_by", data.SortBy)
	base.Set("sort_dir", data.SortDir)
	data.FirstURL = buildEventsPageURL(base, 1)
	data.LastURL = buildEventsPageURL(base, data.TotalPages)
	data.PrevURL = buildEventsPageURL(base, data.Page-1)
	data.NextURL = buildEventsPageURL(base, data.Page+1)

	h.renderer.render(w, "events.html", data)
}

func formatOptionalInt64(v int64) string {
	if v <= 0 {
		return ""
	}
	return strconv.FormatInt(v, 10)
}

func parseDateAtLocalStart(raw string) (time.Time, bool) {
	d, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return d.UTC(), true
}

func parseDateAtLocalEndExclusive(raw string) (time.Time, bool) {
	d, ok := parseDateAtLocalStart(raw)
	if !ok {
		return time.Time{}, false
	}
	return d.Add(24 * time.Hour), true
}

func cloneURLValues(src url.Values) url.Values {
	dst := make(url.Values, len(src))
	for k, values := range src {
		cp := make([]string, len(values))
		copy(cp, values)
		dst[k] = cp
	}
	return dst
}

func buildEventsPageURL(base url.Values, page int) string {
	v := cloneURLValues(base)
	if page < 1 {
		page = 1
	}
	v.Set("page", strconv.Itoa(page))
	return "/admin/events?" + v.Encode()
}

func (h *Handler) loadConnectorNames(ctx context.Context) map[int64]string {
	connectors, err := h.store.ListConnectors(ctx)
	if err != nil {
		return map[int64]string{}
	}
	names := make(map[int64]string, len(connectors))
	for _, c := range connectors {
		names[c.ID] = c.Name
	}
	return names
}

func connectorDisplayName(names map[int64]string, connectorID int64) string {
	if name := strings.TrimSpace(names[connectorID]); name != "" {
		return name
	}
	if connectorID <= 0 {
		return ""
	}
	return strconv.FormatInt(connectorID, 10)
}
