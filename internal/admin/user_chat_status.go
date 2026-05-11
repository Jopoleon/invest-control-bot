package admin

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/chatstatus"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

type adminChatStatusTarget struct {
	user            domain.User
	account         domain.UserMessengerAccount
	sub             domain.Subscription
	connector       domain.Connector
	messengerKind   domain.MessengerKind
	chatRef         string
	messengerUserID string
}

func (h *Handler) refreshUserChatStatus(w http.ResponseWriter, r *http.Request) {
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
	kind := domain.MessengerKind(strings.TrimSpace(r.FormValue("messenger_kind")))
	if h.chatStatus == nil {
		h.renderUserDetailForIDs(r.Context(), w, r, lang, userID, telegramID, t(lang, "users.chat_access.refresh_unavailable"))
		return
	}
	target, ok, err := h.resolveChatStatusTarget(r.Context(), userID, telegramID, subID, kind)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	if !ok {
		h.renderUserDetailForIDs(r.Context(), w, r, lang, userID, telegramID, t(lang, "users.chat_access.target_unavailable"))
		return
	}

	state, err := h.chatStatus.Refresh(r.Context(), chatstatus.RefreshRequest{
		MessengerKind:   target.messengerKind,
		ChatRef:         target.chatRef,
		MessengerUserID: target.messengerUserID,
		UserID:          target.user.ID,
		ConnectorID:     target.connector.ID,
		SubscriptionID:  target.sub.ID,
		Source:          "admin_user_card_refresh",
	})
	details := fmt.Sprintf("subscription_id=%d;connector_id=%d;messenger=%s;chat_ref=%s;messenger_user_id=%s;status=%s",
		target.sub.ID, target.connector.ID, target.messengerKind, target.chatRef, target.messengerUserID, state.Status)
	if err != nil {
		h.logAdminTargetAuditForAccount(r, target.user, target.account, target.connector.ID, domain.AuditActionAdminChatStatusRefreshFailed, details+";"+formatAuditDetail("reason", err.Error(), 240))
		h.renderResolvedUserDetailPage(r.Context(), w, r, lang, target.user, t(lang, "users.chat_access.refresh_failed")+": "+err.Error())
		return
	}
	h.logAdminTargetAuditForAccount(r, target.user, target.account, target.connector.ID, domain.AuditActionAdminChatStatusRefreshed, details)
	h.renderResolvedUserDetailPage(r.Context(), w, r, lang, target.user, t(lang, "users.chat_access.refreshed"))
}

func (h *Handler) resolveChatStatusTarget(ctx context.Context, userID, telegramID, subID int64, kind domain.MessengerKind) (adminChatStatusTarget, bool, error) {
	var target adminChatStatusTarget
	if subID <= 0 || (kind != domain.MessengerKindTelegram && kind != domain.MessengerKindMAX) {
		return target, false, nil
	}
	user, foundUser, err := h.resolveUser(ctx, userID, telegramID)
	if err != nil {
		return target, false, err
	}
	if !foundUser {
		return target, false, nil
	}
	account, foundAccount, err := h.resolveMessengerAccount(ctx, user.ID, kind)
	if err != nil {
		return target, false, err
	}
	if !foundAccount {
		return target, false, nil
	}
	sub, foundSub, err := h.store.GetSubscriptionByID(ctx, subID)
	if err != nil {
		return target, false, err
	}
	if !foundSub || sub.UserID != user.ID {
		return target, false, nil
	}
	connector, foundConnector, err := h.store.GetConnector(ctx, sub.ConnectorID)
	if err != nil {
		return target, false, err
	}
	if !foundConnector {
		return target, false, nil
	}
	chatRef, ok := resolveConnectorChatRef(connector, kind)
	if !ok {
		return target, false, nil
	}
	target = adminChatStatusTarget{
		user:            user,
		account:         account,
		sub:             sub,
		connector:       connector,
		messengerKind:   kind,
		chatRef:         chatRef,
		messengerUserID: strings.TrimSpace(account.MessengerUserID),
	}
	return target, true, nil
}

func (h *Handler) buildChatAccessViews(ctx context.Context, lang string, user domain.User, accounts []domain.UserMessengerAccount, subs []domain.Subscription, now time.Time) []chatAccessView {
	accountsByKind := make(map[domain.MessengerKind]domain.UserMessengerAccount, len(accounts))
	for _, account := range accounts {
		if strings.TrimSpace(account.MessengerUserID) == "" {
			continue
		}
		accountsByKind[account.MessengerKind] = account
	}
	seen := make(map[string]struct{})
	rows := make([]chatAccessView, 0)
	for _, sub := range subs {
		connector, found, err := h.store.GetConnector(ctx, sub.ConnectorID)
		if err != nil || !found {
			continue
		}
		for _, kind := range []domain.MessengerKind{domain.MessengerKindTelegram, domain.MessengerKindMAX} {
			account, hasAccount := accountsByKind[kind]
			if !hasAccount {
				continue
			}
			chatRef, ok := resolveConnectorChatRef(connector, kind)
			if !ok {
				continue
			}
			key := string(kind) + "|" + chatRef + "|" + account.MessengerUserID
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			status, foundStatus, _ := h.store.GetMessengerChatUser(ctx, kind, chatRef, account.MessengerUserID)
			rows = append(rows, h.buildChatAccessView(ctx, lang, user.ID, sub, connector, account, kind, chatRef, status, foundStatus, now))
		}
	}
	return rows
}

func (h *Handler) buildChatAccessView(ctx context.Context, lang string, userID int64, sub domain.Subscription, connector domain.Connector, account domain.UserMessengerAccount, kind domain.MessengerKind, chatRef string, status domain.MessengerChatUser, foundStatus bool, now time.Time) chatAccessView {
	statusValue := domain.MessengerChatUserStatusUnknown
	checkedAt := t(lang, "users.chat_access.not_checked")
	lastError := ""
	if foundStatus {
		statusValue = status.Status
		if !status.CheckedAt.IsZero() {
			checkedAt = status.CheckedAt.In(time.Local).Format("2006-01-02 15:04:05")
		}
		lastError = strings.TrimSpace(status.LastError)
	}
	statusLabel, statusClass := chatStatusBadge(lang, statusValue)
	entitlementLabel, entitlementClass := subscriptionStatusBadgeAt(lang, sub, now)
	canSendAccessLink := kind == domain.MessengerKindTelegram && h.canSendTelegramAccessLink(ctx, userID, sub.ConnectorID, sub.ID, sub.Status)
	return chatAccessView{
		UserID:             userID,
		SubscriptionID:     sub.ID,
		ConnectorID:        connector.ID,
		Connector:          connector.Name,
		MessengerKind:      string(kind),
		MessengerLabel:     messengerKindLabel(lang, kind),
		ChatRef:            chatRef,
		MessengerUserID:    strings.TrimSpace(account.MessengerUserID),
		EntitlementLabel:   entitlementLabel,
		EntitlementClass:   entitlementClass,
		Status:             string(statusValue),
		StatusLabel:        statusLabel,
		StatusClass:        statusClass,
		CheckedAt:          checkedAt,
		LastError:          lastError,
		RefreshURL:         buildChatStatusRefreshURL(lang, userID),
		CanRefresh:         h.chatStatus != nil,
		CanSendAccessLink:  canSendAccessLink,
		AccessLinkURL:      buildSubscriptionAccessLinkURL(lang, userID, sub.ID),
		CanUnbanAccessLink: canSendAccessLink && statusValue == domain.MessengerChatUserStatusBanned,
		UnbanAccessLinkURL: buildSubscriptionUnbanAccessLinkURL(lang, userID, sub.ID),
	}
}

func resolveConnectorChatRef(connector domain.Connector, kind domain.MessengerKind) (string, bool) {
	switch kind {
	case domain.MessengerKindTelegram:
		chatRef := strings.TrimSpace(connector.ResolvedTelegramChatRef())
		return chatRef, chatRef != ""
	case domain.MessengerKindMAX:
		chatID, ok := connector.ResolvedMAXChatID()
		if !ok {
			return "", false
		}
		return strconv.FormatInt(chatID, 10), true
	default:
		return "", false
	}
}

func buildChatStatusRefreshURL(lang string, userID int64) string {
	params := url.Values{}
	params.Set("lang", lang)
	if userID > 0 {
		params.Set("user_id", strconv.FormatInt(userID, 10))
	}
	return "/admin/users/refresh-chat-status?" + params.Encode()
}

func chatStatusBadge(lang string, status domain.MessengerChatUserStatus) (string, string) {
	switch status {
	case domain.MessengerChatUserStatusMember:
		return t(lang, "users.chat_status.member"), "is-success"
	case domain.MessengerChatUserStatusAdministrator:
		return t(lang, "users.chat_status.administrator"), "is-success"
	case domain.MessengerChatUserStatusOwner:
		return t(lang, "users.chat_status.owner"), "is-success"
	case domain.MessengerChatUserStatusRestricted:
		return t(lang, "users.chat_status.restricted"), "is-warning"
	case domain.MessengerChatUserStatusLeft:
		return t(lang, "users.chat_status.left"), "is-muted"
	case domain.MessengerChatUserStatusNotMember:
		return t(lang, "users.chat_status.not_member"), "is-warning"
	case domain.MessengerChatUserStatusBanned:
		return t(lang, "users.chat_status.banned"), "is-danger"
	case domain.MessengerChatUserStatusError:
		return t(lang, "users.chat_status.error"), "is-danger"
	default:
		return t(lang, "users.chat_status.unknown"), "is-muted"
	}
}
