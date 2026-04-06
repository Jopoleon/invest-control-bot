package admin

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
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
		userID, telegramID := parseUserDetailParams(r.FormValue("user_id"), r.FormValue("telegram_id"))
		h.renderUserDetailForIDs(r.Context(), w, r, lang, userID, telegramID, t(lang, "csrf.invalid"))
		return
	}

	userID, telegramID := parseUserDetailParams(r.FormValue("user_id"), r.FormValue("telegram_id"))
	user, found, err := h.resolveUser(r.Context(), userID, telegramID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	account, hasDeliveryAccount, err := h.resolvePreferredMessengerAccount(r.Context(), user.ID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	if !found || !hasDeliveryAccount {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.invalid_id"))
		return
	}
	text := strings.TrimSpace(r.FormValue("message"))
	if text == "" {
		h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, t(lang, "users.actions.message_empty"))
		return
	}

	now := time.Now().UTC()
	if err := h.sendViaMessengerAccount(r.Context(), account, messenger.OutgoingMessage{Text: text}); err != nil {
		h.logAdminTargetAuditForAccount(r, user, account, 0, domain.AuditActionAdminMessageSendFailed, formatAuditDetail("reason", err.Error(), 240))
		h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, err.Error())
		return
	}

	_ = now
	h.logAdminTargetAuditForAccount(r, user, account, 0, domain.AuditActionAdminMessageSent, formatAuditDetail("message_preview", text, 500))
	h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, t(lang, "users.actions.message_sent"))
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
		userID, telegramID := parseUserDetailParams(r.FormValue("user_id"), r.FormValue("telegram_id"))
		h.renderUserDetailForIDs(r.Context(), w, r, lang, userID, telegramID, t(lang, "csrf.invalid"))
		return
	}

	userID, telegramID := parseUserDetailParams(r.FormValue("user_id"), r.FormValue("telegram_id"))
	subID := parseInt64Default(r.FormValue("subscription_id"))
	connectorID := parseInt64Default(r.FormValue("connector_id"))
	user, foundUser, err := h.resolveUser(r.Context(), userID, telegramID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	account, hasDeliveryAccount, err := h.resolvePreferredMessengerAccount(r.Context(), user.ID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	if !foundUser || !hasDeliveryAccount || (subID <= 0 && connectorID <= 0) {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.invalid_id"))
		return
	}

	var (
		sub       domain.Subscription
		ok        bool
		connector domain.Connector
		found     bool
	)
	if subID > 0 {
		sub, ok, err = h.store.GetSubscriptionByID(r.Context(), subID)
		if err != nil || !ok || sub.UserID != user.ID {
			h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, t(lang, "users.revoke.not_found"))
			return
		}
		connectorID = sub.ConnectorID
	}

	connector, found, err = h.store.GetConnector(r.Context(), connectorID)
	if err != nil || !found {
		h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, t(lang, "users.actions.paylink_unavailable"))
		return
	}
	msg, ok := h.buildPaymentLinkMessage(lang, account, connector)
	if !ok {
		h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, t(lang, "users.actions.paylink_unavailable"))
		return
	}
	now := time.Now().UTC()
	if err := h.sendViaMessengerAccount(r.Context(), account, msg); err != nil {
		_ = now
		h.logAdminTargetAuditForAccount(r, user, account, connector.ID, domain.AuditActionAdminPaymentLinkSendFailed, formatAuditDetail("reason", err.Error(), 240))
		h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, err.Error())
		return
	}

	_ = now
	h.logAdminTargetAuditForAccount(r, user, account, connector.ID, domain.AuditActionAdminPaymentLinkSent, "subscription_id="+strconv.FormatInt(subID, 10)+";connector_id="+strconv.FormatInt(connectorID, 10))
	h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, t(lang, "users.actions.paylink_sent"))
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
		userID, telegramID := parseUserDetailParams(r.FormValue("user_id"), r.FormValue("telegram_id"))
		h.renderUserDetailForIDs(r.Context(), w, r, lang, userID, telegramID, t(lang, "csrf.invalid"))
		return
	}

	userID, telegramID := parseUserDetailParams(r.FormValue("user_id"), r.FormValue("telegram_id"))
	subID := parseInt64Default(r.FormValue("subscription_id"))
	user, foundUser, err := h.resolveUser(r.Context(), userID, telegramID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	telegramID, _, hasTelegramIdentity, err := h.resolveTelegramIdentity(r.Context(), user.ID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	if !foundUser || !hasTelegramIdentity || subID <= 0 {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.invalid_id"))
		return
	}

	sub, ok, err := h.store.GetSubscriptionByID(r.Context(), subID)
	if err != nil || !ok || sub.UserID != user.ID {
		h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, t(lang, "users.revoke.not_found"))
		return
	}
	if sub.Status != domain.SubscriptionStatusActive {
		h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, t(lang, "users.revoke.only_active"))
		return
	}

	now := time.Now().UTC()
	if err := h.store.UpdateSubscriptionStatus(r.Context(), sub.ID, domain.SubscriptionStatusRevoked, now); err != nil {
		h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, t(lang, "users.revoke.failed"))
		return
	}

	connector, connectorFound, err := h.store.GetConnector(r.Context(), sub.ConnectorID)
	if err != nil {
		h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, t(lang, "users.revoke.failed"))
		return
	}

	if connectorFound {
		if chatRef := connector.ResolvedTelegramChatRef(); chatRef != "" {
			if err := h.tg.RemoveChatMember(r.Context(), chatRef, telegramID); err != nil {
				_ = now
				h.logAdminTargetAudit(r, user, sub.ConnectorID, domain.AuditActionAdminSubscriptionRevokeFailed, fmt.Sprintf("subscription_id=%d;reason=remove_chat_member_failed", sub.ID))
			} else {
				_ = now
				h.logAdminTargetAudit(r, user, sub.ConnectorID, domain.AuditActionAdminSubscriptionRevokedChat, fmt.Sprintf("subscription_id=%d", sub.ID))
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
	if err := h.tg.SendMessage(r.Context(), telegramID, notifyText, keyboard); err != nil {
		slog.Error("send admin revoke notification failed", "error", err, "subscription_id", sub.ID, "user_id", user.ID)
	}
	_ = now
	h.logAdminTargetAudit(r, user, sub.ConnectorID, domain.AuditActionAdminSubscriptionRevoked, fmt.Sprintf("subscription_id=%d", sub.ID))

	h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, t(lang, "users.revoke.success"))
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
		userID, telegramID := parseUserDetailParams(r.FormValue("user_id"), r.FormValue("telegram_id"))
		h.renderUserDetailForIDs(r.Context(), w, r, lang, userID, telegramID, t(lang, "csrf.invalid"))
		return
	}

	userID, telegramID := parseUserDetailParams(r.FormValue("user_id"), r.FormValue("telegram_id"))
	subID := parseInt64Default(r.FormValue("subscription_id"))
	user, foundUser, err := h.resolveUser(r.Context(), userID, telegramID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	telegramID, _, hasTelegramIdentity, err := h.resolveTelegramIdentity(r.Context(), user.ID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	if !foundUser || !hasTelegramIdentity || subID <= 0 {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.invalid_id"))
		return
	}
	if h.retriggerRebill == nil {
		h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, t(lang, "users.actions.rebill_unavailable"))
		return
	}

	sub, ok, err := h.store.GetSubscriptionByID(r.Context(), subID)
	if err != nil || !ok || sub.UserID != user.ID {
		h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, t(lang, "users.revoke.not_found"))
		return
	}

	result, err := h.retriggerRebill(r.Context(), subID)
	if err != nil {
		h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, err.Error())
		return
	}

	notice := t(lang, "users.actions.rebill_sent")
	if result.Existing {
		notice = t(lang, "users.actions.rebill_existing")
	}
	h.renderResolvedUserDetailPage(r.Context(), w, r, lang, user, notice)
}
