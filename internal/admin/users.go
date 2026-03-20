package admin

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/go-telegram/bot/models"
)

// usersPage renders admin user registry with basic filters.
func (h *Handler) usersPage(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)

	data := usersPageData{
		basePageData: basePageData{
			Lang:       lang,
			I18N:       dictForLang(lang),
			CSRFToken:  h.ensureCSRFToken(w, r),
			TopbarPath: "/admin/users",
			ActiveNav:  "users",
		},
		TelegramID: strings.TrimSpace(r.URL.Query().Get("telegram_id")),
		Search:     strings.TrimSpace(r.URL.Query().Get("search")),
		ExportURL:  buildExportURL("/admin/users/export.csv", r.URL.Query(), lang),
	}

	query := domain.UserListQuery{Limit: 300, Search: data.Search}
	if data.TelegramID != "" {
		if id, err := strconv.ParseInt(data.TelegramID, 10, 64); err == nil && id > 0 {
			query.TelegramID = id
		}
	}

	users, err := h.store.ListUsers(r.Context(), query)
	if err != nil {
		data.Notice = t(lang, "users.load_error")
		h.renderer.render(w, "users.html", data)
		return
	}

	data.Users = make([]userView, 0, len(users))
	for _, user := range users {
		autoPayLabel, autoPayClass := autoPayBadge(lang, user.AutoPayEnabled, user.HasAutoPaySettings)
		data.Users = append(data.Users, userView{
			TelegramID:       user.TelegramID,
			TelegramUsername: user.TelegramUsername,
			FullName:         user.FullName,
			Phone:            user.Phone,
			Email:            user.Email,
			AutoPay:          autoPayLabel,
			AutoPayClass:     autoPayClass,
			UpdatedAt:        user.UpdatedAt.In(time.Local).Format("2006-01-02 15:04:05"),
			DetailURL:        buildUserDetailURL(lang, user.TelegramID),
		})
	}

	h.renderer.render(w, "users.html", data)
}

// userDetailPage renders one user profile with linked subscriptions/payments/audit history.
func (h *Handler) userDetailPage(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)
	rawTelegramID := strings.TrimSpace(r.URL.Query().Get("telegram_id"))
	telegramID, err := strconv.ParseInt(rawTelegramID, 10, 64)
	if err != nil || telegramID <= 0 {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.invalid_id"))
		return
	}
	h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, strings.TrimSpace(r.URL.Query().Get("notice")))
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
	item, found, err := h.store.GetUser(ctx, telegramID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	if !found {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.not_found"))
		return
	}

	autoPayEnabled, hasAutoPaySettings, err := h.store.GetUserAutoPayEnabled(ctx, telegramID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}

	payments, err := h.store.ListPayments(ctx, domain.PaymentListQuery{
		TelegramID: telegramID,
		Limit:      200,
	})
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}

	subs, err := h.store.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		TelegramID: telegramID,
		Limit:      200,
	})
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}

	events, _, err := h.store.ListAuditEvents(ctx, domain.AuditEventListQuery{
		TelegramID: telegramID,
		SortBy:     "created_at",
		SortDesc:   true,
		Page:       1,
		PageSize:   200,
	})
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}

	connectorNames := h.loadConnectorNames(ctx)
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
		User: userView{
			TelegramID:       item.TelegramID,
			TelegramUsername: item.TelegramUsername,
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
		canSendPayLink := false
		if connector, found, err := h.store.GetConnector(ctx, s.ConnectorID); err == nil && found {
			canSendPayLink = buildAdminBotStartURL(h.botUsername, connector.StartPayload) != ""
		}
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
			CanRevoke:      s.Status == domain.SubscriptionStatusActive,
			RevokeURL:      buildSubscriptionRevokeURL(lang, telegramID, s.ID),
			CanSendPayLink: canSendPayLink,
			PaymentLinkURL: buildUserPaymentLinkURL(lang, telegramID, s.ID),
		})
	}

	data.Events = make([]auditEventView, 0, len(events))
	for _, event := range events {
		data.Events = append(data.Events, auditEventView{
			CreatedAt:   event.CreatedAt.In(time.Local).Format("2006-01-02 15:04:05"),
			TelegramID:  event.TelegramID,
			ConnectorID: event.ConnectorID,
			Connector:   connectorDisplayName(connectorNames, event.ConnectorID),
			Action:      event.Action,
			Details:     event.Details,
		})
	}

	h.renderer.render(w, "user_detail.html", data)
}

func (h *Handler) sendUserMessage(w http.ResponseWriter, r *http.Request) {
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
		h.unauthorized(w)
		return
	}
	if !h.verifyCSRF(r) {
		w.WriteHeader(http.StatusForbidden)
		h.renderUserDetailPage(r.Context(), w, r, lang, parseInt64Default(r.FormValue("telegram_id")), t(lang, "csrf.invalid"))
		return
	}

	telegramID := parseInt64Default(r.FormValue("telegram_id"))
	if telegramID <= 0 {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.invalid_id"))
		return
	}
	text := strings.TrimSpace(r.FormValue("message"))
	if text == "" {
		h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.actions.message_empty"))
		return
	}

	now := time.Now().UTC()
	if err := h.tg.SendMessage(r.Context(), telegramID, text, nil); err != nil {
		_ = h.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
			TelegramID: telegramID,
			Action:     "admin_message_send_failed",
			Details:    err.Error(),
			CreatedAt:  now,
		})
		h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, err.Error())
		return
	}

	_ = h.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
		TelegramID: telegramID,
		Action:     "admin_message_sent",
		Details:    trimAuditDetails(text, 500),
		CreatedAt:  now,
	})
	h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.actions.message_sent"))
}

func (h *Handler) sendUserPaymentLink(w http.ResponseWriter, r *http.Request) {
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
		h.unauthorized(w)
		return
	}
	if !h.verifyCSRF(r) {
		w.WriteHeader(http.StatusForbidden)
		h.renderUserDetailPage(r.Context(), w, r, lang, parseInt64Default(r.FormValue("telegram_id")), t(lang, "csrf.invalid"))
		return
	}

	telegramID := parseInt64Default(r.FormValue("telegram_id"))
	subID := parseInt64Default(r.FormValue("subscription_id"))
	connectorID := parseInt64Default(r.FormValue("connector_id"))
	if telegramID <= 0 || (subID <= 0 && connectorID <= 0) {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.invalid_id"))
		return
	}

	var (
		sub       domain.Subscription
		ok        bool
		err       error
		connector domain.Connector
		found     bool
	)
	if subID > 0 {
		sub, ok, err = h.store.GetSubscriptionByID(r.Context(), subID)
		if err != nil || !ok || sub.TelegramID != telegramID {
			h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.revoke.not_found"))
			return
		}
		connectorID = sub.ConnectorID
	}

	connector, found, err = h.store.GetConnector(r.Context(), connectorID)
	if err != nil || !found {
		h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.actions.paylink_unavailable"))
		return
	}
	renewURL := buildAdminBotStartURL(h.botUsername, connector.StartPayload)
	if renewURL == "" {
		h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.actions.paylink_unavailable"))
		return
	}

	text := t(lang, "users.actions.paylink_text")
	if strings.TrimSpace(connector.Name) != "" {
		text = fmt.Sprintf(t(lang, "users.actions.paylink_text_named"), connector.Name)
	}
	keyboard := &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{{
			{Text: t(lang, "users.actions.paylink_button"), URL: renewURL},
		}},
	}
	now := time.Now().UTC()
	if err := h.tg.SendMessage(r.Context(), telegramID, text, keyboard); err != nil {
		_ = h.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
			TelegramID:  telegramID,
			ConnectorID: connector.ID,
			Action:      "admin_payment_link_send_failed",
			Details:     err.Error(),
			CreatedAt:   now,
		})
		h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, err.Error())
		return
	}

	_ = h.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
		TelegramID:  telegramID,
		ConnectorID: connector.ID,
		Action:      "admin_payment_link_sent",
		Details:     "subscription_id=" + strconv.FormatInt(subID, 10) + ",connector_id=" + strconv.FormatInt(connectorID, 10),
		CreatedAt:   now,
	})
	h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.actions.paylink_sent"))
}

// revokeSubscription performs manual admin-side subscription revocation.
func (h *Handler) revokeSubscription(w http.ResponseWriter, r *http.Request) {
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
		h.unauthorized(w)
		return
	}
	if !h.verifyCSRF(r) {
		w.WriteHeader(http.StatusForbidden)
		h.renderUserDetailPage(r.Context(), w, r, lang, parseInt64Default(r.FormValue("telegram_id")), t(lang, "csrf.invalid"))
		return
	}

	telegramID := parseInt64Default(r.FormValue("telegram_id"))
	subID := parseInt64Default(r.FormValue("subscription_id"))
	if telegramID <= 0 || subID <= 0 {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.invalid_id"))
		return
	}

	sub, ok, err := h.store.GetSubscriptionByID(r.Context(), subID)
	if err != nil || !ok || sub.TelegramID != telegramID {
		h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.revoke.not_found"))
		return
	}
	if sub.Status != domain.SubscriptionStatusActive {
		h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.revoke.only_active"))
		return
	}

	now := time.Now().UTC()
	if err := h.store.UpdateSubscriptionStatus(r.Context(), sub.ID, domain.SubscriptionStatusRevoked, now); err != nil {
		h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.revoke.failed"))
		return
	}

	connector, connectorFound, err := h.store.GetConnector(r.Context(), sub.ConnectorID)
	if err != nil {
		h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.revoke.failed"))
		return
	}

	if connectorFound {
		if chatID, ok := normalizeAdminTelegramChatID(connector.ChatID); ok {
			if err := h.tg.RemoveChatMember(r.Context(), chatID, telegramID); err != nil {
				_ = h.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
					TelegramID:  telegramID,
					ConnectorID: sub.ConnectorID,
					Action:      "admin_subscription_revoke_chat_failed",
					Details:     fmt.Sprintf("subscription_id=%d", sub.ID),
					CreatedAt:   now,
				})
			} else {
				_ = h.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
					TelegramID:  telegramID,
					ConnectorID: sub.ConnectorID,
					Action:      "admin_subscription_revoked_from_chat",
					Details:     fmt.Sprintf("subscription_id=%d", sub.ID),
					CreatedAt:   now,
				})
			}
		}
	}

	notifyText := t(lang, "users.revoke.notify")
	var keyboard *models.InlineKeyboardMarkup
	if connectorFound {
		notifyText = fmt.Sprintf(t(lang, "users.revoke.notify_with_connector"), connector.Name)
		if renewURL := buildAdminBotStartURL(h.botUsername, connector.StartPayload); renewURL != "" {
			keyboard = &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{{
					{Text: t(lang, "users.revoke.renew"), URL: renewURL},
				}},
			}
		}
	}
	_ = h.tg.SendMessage(r.Context(), telegramID, notifyText, keyboard)
	_ = h.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
		TelegramID:  telegramID,
		ConnectorID: sub.ConnectorID,
		Action:      "admin_subscription_revoked",
		Details:     fmt.Sprintf("subscription_id=%d", sub.ID),
		CreatedAt:   now,
	})

	h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.revoke.success"))
}

func buildUserDetailURL(lang string, telegramID int64) string {
	params := url.Values{}
	params.Set("lang", lang)
	params.Set("telegram_id", strconv.FormatInt(telegramID, 10))
	return "/admin/users/view?" + params.Encode()
}

func buildSubscriptionRevokeURL(lang string, telegramID, subscriptionID int64) string {
	params := url.Values{}
	params.Set("lang", lang)
	params.Set("telegram_id", strconv.FormatInt(telegramID, 10))
	params.Set("subscription_id", strconv.FormatInt(subscriptionID, 10))
	return "/admin/subscriptions/revoke?" + params.Encode()
}

func buildUserPaymentLinkURL(lang string, telegramID, subscriptionID int64) string {
	params := url.Values{}
	params.Set("lang", lang)
	params.Set("telegram_id", strconv.FormatInt(telegramID, 10))
	params.Set("subscription_id", strconv.FormatInt(subscriptionID, 10))
	return "/admin/users/send-payment-link?" + params.Encode()
}

func parseInt64Default(raw string) int64 {
	value, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return value
}

func normalizeAdminTelegramChatID(chatIDRaw string) (int64, bool) {
	raw := strings.TrimSpace(chatIDRaw)
	if raw == "" {
		return 0, false
	}
	raw = strings.TrimPrefix(raw, "+")
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value == 0 {
		return 0, false
	}
	if value < 0 {
		return value, true
	}
	return -value, true
}

func buildAdminBotStartURL(botUsername, startPayload string) string {
	username := strings.TrimSpace(strings.TrimPrefix(botUsername, "@"))
	payload := strings.TrimSpace(startPayload)
	if username == "" || payload == "" {
		return ""
	}
	return "https://t.me/" + username + "?start=" + payload
}

func trimAuditDetails(raw string, limit int) string {
	text := strings.TrimSpace(raw)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	if limit <= 3 {
		return text[:limit]
	}
	return text[:limit-3] + "..."
}
