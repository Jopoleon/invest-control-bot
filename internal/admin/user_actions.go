package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/go-telegram/bot/models"
)

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
			Action:     domain.AuditActionAdminMessageSendFailed,
			Details:    err.Error(),
			CreatedAt:  now,
		})
		h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, err.Error())
		return
	}

	_ = h.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
		TelegramID: telegramID,
		Action:     domain.AuditActionAdminMessageSent,
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
			Action:      domain.AuditActionAdminPaymentLinkSendFailed,
			Details:     err.Error(),
			CreatedAt:   now,
		})
		h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, err.Error())
		return
	}

	_ = h.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
		TelegramID:  telegramID,
		ConnectorID: connector.ID,
		Action:      domain.AuditActionAdminPaymentLinkSent,
		Details:     "subscription_id=" + strconv.FormatInt(subID, 10) + ",connector_id=" + strconv.FormatInt(connectorID, 10),
		CreatedAt:   now,
	})
	h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.actions.paylink_sent"))
}

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
					Action:      domain.AuditActionAdminSubscriptionRevokeFailed,
					Details:     fmt.Sprintf("subscription_id=%d", sub.ID),
					CreatedAt:   now,
				})
			} else {
				_ = h.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
					TelegramID:  telegramID,
					ConnectorID: sub.ConnectorID,
					Action:      domain.AuditActionAdminSubscriptionRevokedChat,
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
		Action:      domain.AuditActionAdminSubscriptionRevoked,
		Details:     fmt.Sprintf("subscription_id=%d", sub.ID),
		CreatedAt:   now,
	})

	h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.revoke.success"))
}

func (h *Handler) triggerSubscriptionRebill(w http.ResponseWriter, r *http.Request) {
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
	if h.retriggerRebill == nil {
		h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.actions.rebill_unavailable"))
		return
	}

	sub, ok, err := h.store.GetSubscriptionByID(r.Context(), subID)
	if err != nil || !ok || sub.TelegramID != telegramID {
		h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, t(lang, "users.revoke.not_found"))
		return
	}

	result, err := h.retriggerRebill(r.Context(), subID)
	if err != nil {
		h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, err.Error())
		return
	}

	notice := t(lang, "users.actions.rebill_sent")
	if result.Existing {
		notice = t(lang, "users.actions.rebill_existing")
	}
	h.renderUserDetailPage(r.Context(), w, r, lang, telegramID, notice)
}
