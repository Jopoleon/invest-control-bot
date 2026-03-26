package admin

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

// billingPage renders payments/subscriptions with admin filters.
func (h *Handler) billingPage(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)

	data := billingPageData{
		basePageData: basePageData{
			Lang:       lang,
			I18N:       dictForLang(lang),
			CSRFToken:  h.ensureCSRFToken(w, r),
			TopbarPath: "/admin/billing",
			ActiveNav:  "billing",
		},
		PaymentStatus:          strings.TrimSpace(r.URL.Query().Get("payment_status")),
		SubscriptionStatus:     strings.TrimSpace(r.URL.Query().Get("subscription_status")),
		TelegramID:             strings.TrimSpace(r.URL.Query().Get("telegram_id")),
		ConnectorID:            strings.TrimSpace(r.URL.Query().Get("connector_id")),
		DateFrom:               strings.TrimSpace(r.URL.Query().Get("date_from")),
		DateTo:                 strings.TrimSpace(r.URL.Query().Get("date_to")),
		PaymentsExportURL:      buildExportURL("/admin/billing/payments/export.csv", r.URL.Query(), lang),
		SubscriptionsExportURL: buildExportURL("/admin/billing/subscriptions/export.csv", r.URL.Query(), lang),
	}

	paymentQuery := domain.PaymentListQuery{Limit: 1000}
	subQuery := domain.SubscriptionListQuery{Limit: 1000}

	telegramID, err := h.resolveFilterTelegramID(r.Context(), r.URL.Query().Get("user_id"), data.TelegramID)
	if err != nil {
		data.Notice = t(lang, "billing.load_error")
		h.renderer.render(w, "billing.html", data)
		return
	}
	if telegramID > 0 {
		paymentQuery.TelegramID = telegramID
		subQuery.TelegramID = telegramID
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
		statusLabel, statusClass := paymentStatusBadge(lang, p.Status)
		autoPayLabel, autoPayClass := autoPayBadge(lang, p.AutoPayEnabled, true)
		data.Payments = append(data.Payments, paymentView{
			ID:                p.ID,
			Provider:          p.Provider,
			ProviderPaymentID: p.ProviderPaymentID,
			Status:            string(p.Status),
			StatusLabel:       statusLabel,
			StatusClass:       statusClass,
			AutoPayEnabled:    p.AutoPayEnabled,
			AutoPayLabel:      autoPayLabel,
			AutoPayClass:      autoPayClass,
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
		statusLabel, statusClass := subscriptionStatusBadge(lang, s.Status)
		autoPayLabel, autoPayClass := autoPayBadge(lang, s.AutoPayEnabled, true)
		data.Subscriptions = append(data.Subscriptions, subscriptionView{
			ID:             s.ID,
			Status:         string(s.Status),
			StatusLabel:    statusLabel,
			StatusClass:    statusClass,
			AutoPayEnabled: s.AutoPayEnabled,
			AutoPayLabel:   autoPayLabel,
			AutoPayClass:   autoPayClass,
			TelegramID:     s.TelegramID,
			ConnectorID:    s.ConnectorID,
			Connector:      connectorDisplayName(connectorNames, s.ConnectorID),
			PaymentID:      s.PaymentID,
			StartsAt:       s.StartsAt.In(time.Local).Format("2006-01-02 15:04:05"),
			EndsAt:         s.EndsAt.In(time.Local).Format("2006-01-02 15:04:05"),
			CreatedAt:      s.CreatedAt.In(time.Local).Format("2006-01-02 15:04:05"),
		})
	}

	data.Summary, data.Groups = buildBillingSummary(payments, subs, connectorNames)

	h.renderer.render(w, "billing.html", data)
}

func buildBillingSummary(payments []domain.Payment, subs []domain.Subscription, connectorNames map[int64]string) (billingSummaryView, []billingGroupView) {
	summary := billingSummaryView{
		TotalPayments: len(payments),
	}
	groupMap := make(map[int64]*billingGroupView)

	for _, payment := range payments {
		group := ensureBillingGroup(groupMap, connectorNames, payment.ConnectorID)
		switch payment.Status {
		case domain.PaymentStatusPaid:
			summary.PaidPayments++
			summary.PaidAmountRUB += payment.AmountRUB
			group.PaidPayments++
			group.PaidAmountRUB += payment.AmountRUB
		case domain.PaymentStatusPending:
			summary.PendingAmountRUB += payment.AmountRUB
			group.PendingPayments++
		case domain.PaymentStatusFailed:
			summary.FailedPayments++
			group.FailedPayments++
		}
	}

	for _, sub := range subs {
		if sub.Status != domain.SubscriptionStatusActive {
			continue
		}
		summary.ActiveSubscriptions++
		group := ensureBillingGroup(groupMap, connectorNames, sub.ConnectorID)
		group.ActiveSubscriptions++
	}

	groups := make([]billingGroupView, 0, len(groupMap))
	for _, group := range groupMap {
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].PaidAmountRUB == groups[j].PaidAmountRUB {
			return groups[i].Connector < groups[j].Connector
		}
		return groups[i].PaidAmountRUB > groups[j].PaidAmountRUB
	})

	return summary, groups
}

func ensureBillingGroup(groups map[int64]*billingGroupView, connectorNames map[int64]string, connectorID int64) *billingGroupView {
	group, ok := groups[connectorID]
	if ok {
		return group
	}
	group = &billingGroupView{
		ConnectorID: connectorID,
		Connector:   connectorDisplayName(connectorNames, connectorID),
	}
	groups[connectorID] = group
	return group
}
