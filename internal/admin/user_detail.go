package admin

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func (h *Handler) userDetailPage(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)
	userID, telegramID := parseUserDetailParams(r.URL.Query().Get("user_id"), r.URL.Query().Get("telegram_id"))
	if userID <= 0 && telegramID <= 0 {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.invalid_id"))
		return
	}
	user, found, err := h.resolveUser(r.Context(), userID, telegramID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	if !found {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.not_found"))
		return
	}
	h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, strings.TrimSpace(r.URL.Query().Get("notice")))
}

func renderUserDetailError(h *Handler, w http.ResponseWriter, r *http.Request, lang, notice string) {
	h.renderer.render(w, "user_detail.html", userDetailPageData{
		basePageData: basePageData{
			Lang:       lang,
			I18N:       dictForLang(lang),
			CSRFToken:  h.ensureCSRFToken(w, r),
			TopbarPath: "/admin/users",
			ActiveNav:  "users",
		},
		Notice:           notice,
		BackURL:          "/admin/users?lang=" + lang,
		MessageActionURL: "/admin/users/message?lang=" + lang,
	})
}

func (h *Handler) renderUserDetailPage(ctx context.Context, w http.ResponseWriter, r *http.Request, lang string, telegramID int64, notice string) {
	item, found, err := h.resolveUser(ctx, 0, telegramID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	if !found {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.not_found"))
		return
	}
	h.renderResolvedUserDetailPage(ctx, w, r, lang, item, notice)
}

func (h *Handler) renderResolvedUserDetailPage(ctx context.Context, w http.ResponseWriter, r *http.Request, lang string, item domain.User, notice string) {
	telegramID, telegramUsername, hasTelegramIdentity, err := h.resolveTelegramIdentity(ctx, item.ID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	payments, err := h.store.ListPayments(ctx, domain.PaymentListQuery{UserID: item.ID, Limit: 200})
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	subs, err := h.store.ListSubscriptions(ctx, domain.SubscriptionListQuery{UserID: item.ID, Limit: 200})
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	events, _, err := h.store.ListAuditEvents(ctx, domain.AuditEventListQuery{
		TargetUserID: item.ID,
		SortBy:       "created_at",
		SortDesc:     true,
		Page:         1,
		PageSize:     200,
	})
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	consents, err := h.store.ListConsentsByUser(ctx, item.ID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	recurringConsents, err := h.store.ListRecurringConsentsByUser(ctx, item.ID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}

	connectorNames := h.loadConnectorNames(ctx)
	resolveTelegramIdentity := h.buildTelegramIdentityLookup(ctx)
	autoPayEnabled, hasAutoPaySettings := summarizeAutopayFromSubscriptions(subs)
	data := userDetailPageData{
		basePageData: basePageData{
			Lang:       lang,
			I18N:       dictForLang(lang),
			CSRFToken:  h.ensureCSRFToken(w, r),
			TopbarPath: "/admin/users",
			ActiveNav:  "users",
		},
		Notice:           notice,
		BackURL:          "/admin/users?lang=" + lang,
		MessageActionURL: "/admin/users/message?lang=" + lang,
		AutopayCancelURL: h.buildAutopayCancelURL(telegramID),
		User: userView{
			UserID:           item.ID,
			TelegramID:       telegramID,
			TelegramUsername: telegramUsername,
			FullName:         item.FullName,
			Phone:            item.Phone,
			Email:            item.Email,
			AutoPay:          func() string { label, _ := autoPayBadge(lang, autoPayEnabled, hasAutoPaySettings); return label }(),
			AutoPayClass: func() string {
				_, className := autoPayBadge(lang, autoPayEnabled, hasAutoPaySettings)
				return className
			}(),
			UpdatedAt: item.UpdatedAt.In(time.Local).Format("2006-01-02 15:04:05"),
		},
	}
	data.RecurringSummary = buildRecurringSummary(lang, autoPayEnabled, hasAutoPaySettings, recurringConsents, connectorNames, payments, subs)

	data.Consents = make([]consentView, 0, len(consents))
	for _, consent := range consents {
		data.Consents = append(data.Consents, consentView{
			Connector:            connectorDisplayName(connectorNames, consent.ConnectorID),
			OfferAcceptedAt:      consent.OfferAcceptedAt.In(time.Local).Format("2006-01-02 15:04:05"),
			OfferDocumentLabel:   consentDocumentLabel(lang, consent.OfferDocumentID, consent.OfferDocumentVersion),
			PrivacyAcceptedAt:    consent.PrivacyAcceptedAt.In(time.Local).Format("2006-01-02 15:04:05"),
			PrivacyDocumentLabel: consentDocumentLabel(lang, consent.PrivacyDocumentID, consent.PrivacyDocumentVersion),
		})
	}

	data.RecurringConsents = make([]recurringConsentView, 0, len(recurringConsents))
	for _, consent := range recurringConsents {
		data.RecurringConsents = append(data.RecurringConsents, recurringConsentView{
			Connector:             connectorDisplayName(connectorNames, consent.ConnectorID),
			AcceptedAt:            consent.AcceptedAt.In(time.Local).Format("2006-01-02 15:04:05"),
			OfferDocumentLabel:    consentDocumentLabel(lang, consent.OfferDocumentID, consent.OfferDocumentVersion),
			UserAgreementDocLabel: consentDocumentLabel(lang, consent.UserAgreementDocumentID, consent.UserAgreementDocumentVersion),
		})
	}

	data.Payments = make([]paymentView, 0, len(payments))
	for _, p := range payments {
		paymentTelegramID, _, _, err := resolveTelegramIdentity(p.UserID)
		if err != nil {
			renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
			return
		}
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
			TelegramID:        paymentTelegramID,
			ConnectorID:       p.ConnectorID,
			Connector:         connectorDisplayName(connectorNames, p.ConnectorID),
			AmountRUB:         p.AmountRUB,
			CreatedAt:         p.CreatedAt.In(time.Local).Format("2006-01-02 15:04:05"),
			PaidAt:            paidAt,
		})
	}

	data.Subscriptions = make([]subscriptionView, 0, len(subs))
	for _, s := range subs {
		subscriptionTelegramID, _, _, err := resolveTelegramIdentity(s.UserID)
		if err != nil {
			renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
			return
		}
		statusLabel, statusClass := subscriptionStatusBadge(lang, s.Status)
		autoPayLabel, autoPayClass := autoPayBadge(lang, s.AutoPayEnabled, true)
		canSendPayLink := false
		if connector, found, err := h.store.GetConnector(ctx, s.ConnectorID); err == nil && found {
			canSendPayLink = buildAdminBotStartURL(h.botUsername, connector.StartPayload) != ""
		}
		data.Subscriptions = append(data.Subscriptions, subscriptionView{
			ID:               s.ID,
			Status:           string(s.Status),
			StatusLabel:      statusLabel,
			StatusClass:      statusClass,
			AutoPayEnabled:   s.AutoPayEnabled,
			AutoPayLabel:     autoPayLabel,
			AutoPayClass:     autoPayClass,
			TelegramID:       subscriptionTelegramID,
			ConnectorID:      s.ConnectorID,
			Connector:        connectorDisplayName(connectorNames, s.ConnectorID),
			PaymentID:        s.PaymentID,
			StartsAt:         s.StartsAt.In(time.Local).Format("2006-01-02 15:04:05"),
			EndsAt:           s.EndsAt.In(time.Local).Format("2006-01-02 15:04:05"),
			CreatedAt:        s.CreatedAt.In(time.Local).Format("2006-01-02 15:04:05"),
			CanRevoke:        s.Status == domain.SubscriptionStatusActive,
			RevokeURL:        buildSubscriptionRevokeURL(lang, item.ID, telegramID, s.ID),
			CanSendPayLink:   canSendPayLink,
			PaymentLinkURL:   buildUserPaymentLinkURL(lang, item.ID, telegramID, s.ID),
			CanTriggerRebill: h.retriggerRebill != nil && s.Status == domain.SubscriptionStatusActive && s.AutoPayEnabled,
			RebillURL:        buildSubscriptionRebillURL(lang, item.ID, telegramID, s.ID),
		})
	}

	data.Events = make([]auditEventView, 0, len(events))
	for _, event := range events {
		data.Events = append(data.Events, auditEventView{
			CreatedAt:             event.CreatedAt.In(time.Local).Format("2006-01-02 15:04:05"),
			ActorType:             string(event.ActorType),
			TargetMessengerUserID: event.TargetMessengerUserID,
			ConnectorID:           event.ConnectorID,
			Connector:             connectorDisplayName(connectorNames, event.ConnectorID),
			Action:                event.Action,
			Details:               event.Details,
		})
	}
	if !hasTelegramIdentity {
		data.AutopayCancelURL = ""
	}

	h.renderer.render(w, "user_detail.html", data)
}

func consentDocumentLabel(lang string, documentID int64, version int) string {
	if documentID <= 0 || version <= 0 {
		return t(lang, "users.consents.custom")
	}
	return t(lang, "users.consents.version_prefix") + " " + strconv.Itoa(version) + " · ID " + strconv.FormatInt(documentID, 10)
}

func buildRecurringSummary(lang string, autoPayEnabled, hasAutoPaySettings bool, recurringConsents []domain.RecurringConsent, connectorNames map[int64]string, payments []domain.Payment, subs []domain.Subscription) recurringSummaryView {
	statusLabel, statusClass := autoPayBadge(lang, autoPayEnabled, hasAutoPaySettings)
	summary := recurringSummaryView{
		StatusLabel:          statusLabel,
		StatusClass:          statusClass,
		LastConsentAt:        "—",
		LastConsentConnector: "—",
		HealthLabel:          t(lang, "users.recurring.health_no_consent"),
		HealthClass:          "is-warning",
		LastRebillLabel:      t(lang, "users.recurring.rebill_none"),
		LastRebillClass:      "is-muted",
		LastRebillAt:         "—",
	}
	if len(recurringConsents) > 0 {
		latest := recurringConsents[0]
		summary.LastConsentAt = latest.AcceptedAt.In(time.Local).Format("2006-01-02 15:04:05")
		summary.LastConsentConnector = connectorDisplayName(connectorNames, latest.ConnectorID)
		summary.HealthLabel = t(lang, "users.recurring.health_consistent")
		summary.HealthClass = "is-success"
	}
	if autoPayEnabled && len(recurringConsents) == 0 {
		summary.HealthLabel = t(lang, "users.recurring.health_missing_consent")
		summary.HealthClass = "is-danger"
	}
	if !autoPayEnabled && len(recurringConsents) > 0 {
		summary.HealthLabel = t(lang, "users.recurring.health_disabled")
		summary.HealthClass = "is-muted"
	}

	for _, sub := range subs {
		if !sub.AutoPayEnabled {
			continue
		}
		state := buildRecurringPaymentState(payments, sub.ID)
		if !state.HasAttempts {
			continue
		}
		summary.FailedAttempts = state.FailedAttempts
		if !state.LastAttemptAt.IsZero() {
			summary.LastRebillAt = state.LastAttemptAt.In(time.Local).Format("2006-01-02 15:04:05")
		}
		switch {
		case state.LastStatus == domain.PaymentStatusPending:
			summary.LastRebillLabel = t(lang, "users.recurring.rebill_pending")
			summary.LastRebillClass = "is-accent"
		case state.FailedAttempts >= 3:
			summary.LastRebillLabel = t(lang, "users.recurring.rebill_failed_exhausted")
			summary.LastRebillClass = "is-danger"
		case state.FailedAttempts > 0:
			summary.LastRebillLabel = t(lang, "users.recurring.rebill_failed")
			summary.LastRebillClass = "is-warning"
		case state.LastStatus == domain.PaymentStatusPaid:
			summary.LastRebillLabel = t(lang, "users.recurring.rebill_paid")
			summary.LastRebillClass = "is-success"
		}
		break
	}
	return summary
}

func summarizeAutopayFromSubscriptions(subs []domain.Subscription) (bool, bool) {
	hasActive := false
	enabled := false
	for _, sub := range subs {
		if sub.Status != domain.SubscriptionStatusActive {
			continue
		}
		hasActive = true
		if sub.AutoPayEnabled {
			enabled = true
		}
	}
	return enabled, hasActive
}
