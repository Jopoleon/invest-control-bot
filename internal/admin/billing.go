package admin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
)

// billingPage renders payments/subscriptions with admin filters.
func (h *Handler) billingPage(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		h.unauthorized(w)
		return
	}
	h.persistTokenCookie(w, r)
	lang := h.resolveLang(w, r)

	data := billingPageData{
		basePageData: basePageData{
			Lang: lang,
			I18N: dictForLang(lang),
		},
		PaymentStatus:      strings.TrimSpace(r.URL.Query().Get("payment_status")),
		SubscriptionStatus: strings.TrimSpace(r.URL.Query().Get("subscription_status")),
		TelegramID:         strings.TrimSpace(r.URL.Query().Get("telegram_id")),
		ConnectorID:        strings.TrimSpace(r.URL.Query().Get("connector_id")),
		DateFrom:           strings.TrimSpace(r.URL.Query().Get("date_from")),
		DateTo:             strings.TrimSpace(r.URL.Query().Get("date_to")),
	}

	paymentQuery := domain.PaymentListQuery{Limit: 300}
	subQuery := domain.SubscriptionListQuery{Limit: 300}

	if data.TelegramID != "" {
		if id, err := strconv.ParseInt(data.TelegramID, 10, 64); err == nil && id > 0 {
			paymentQuery.TelegramID = id
			subQuery.TelegramID = id
		}
	}
	if data.ConnectorID != "" {
		if id, err := strconv.ParseInt(data.ConnectorID, 10, 64); err == nil && id > 0 {
			paymentQuery.ConnectorID = id
			subQuery.ConnectorID = id
		}
	}
	if data.PaymentStatus != "" {
		paymentQuery.Status = domain.PaymentStatus(data.PaymentStatus)
	}
	if data.SubscriptionStatus != "" {
		subQuery.Status = domain.SubscriptionStatus(data.SubscriptionStatus)
	}
	if data.DateFrom != "" {
		if from, ok := parseDateAtLocalStart(data.DateFrom); ok {
			paymentQuery.CreatedFrom = &from
			subQuery.CreatedFrom = &from
		}
	}
	if data.DateTo != "" {
		if to, ok := parseDateAtLocalEndExclusive(data.DateTo); ok {
			paymentQuery.CreatedToExclude = &to
			subQuery.CreatedToExclude = &to
		}
	}

	payments, err := h.store.ListPayments(r.Context(), paymentQuery)
	if err != nil {
		data.Notice = t(lang, "billing.load_error")
		h.renderer.render(w, "billing.html", data)
		return
	}
	subs, err := h.store.ListSubscriptions(r.Context(), subQuery)
	if err != nil {
		data.Notice = t(lang, "billing.load_error")
		h.renderer.render(w, "billing.html", data)
		return
	}
	connectorNames := h.loadConnectorNames(r.Context())

	data.Payments = make([]paymentView, 0, len(payments))
	for _, p := range payments {
		paidAt := ""
		if p.PaidAt != nil {
			paidAt = p.PaidAt.In(time.Local).Format("2006-01-02 15:04:05")
		}
		data.Payments = append(data.Payments, paymentView{
			ID:                p.ID,
			Provider:          p.Provider,
			ProviderPaymentID: p.ProviderPaymentID,
			Status:            string(p.Status),
			TelegramID:        p.TelegramID,
			ConnectorID:       p.ConnectorID,
			Connector:         connectorDisplayName(connectorNames, p.ConnectorID),
			AmountRUB:         p.AmountRUB,
			CreatedAt:         p.CreatedAt.In(time.Local).Format("2006-01-02 15:04:05"),
			PaidAt:            paidAt,
		})
	}

	data.Subscriptions = make([]subscriptionView, 0, len(subs))
	for _, s := range subs {
		data.Subscriptions = append(data.Subscriptions, subscriptionView{
			ID:          s.ID,
			Status:      string(s.Status),
			TelegramID:  s.TelegramID,
			ConnectorID: s.ConnectorID,
			Connector:   connectorDisplayName(connectorNames, s.ConnectorID),
			PaymentID:   s.PaymentID,
			StartsAt:    s.StartsAt.In(time.Local).Format("2006-01-02 15:04:05"),
			EndsAt:      s.EndsAt.In(time.Local).Format("2006-01-02 15:04:05"),
			CreatedAt:   s.CreatedAt.In(time.Local).Format("2006-01-02 15:04:05"),
		})
	}

	h.renderer.render(w, "billing.html", data)
}
