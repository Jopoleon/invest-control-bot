package admin

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

type churnIssueKind string

const (
	churnIssueFailedPayment       churnIssueKind = "failed_payment"
	churnIssueExpiredSubscription churnIssueKind = "expired_subscription"
	churnIssueRevokedSubscription churnIssueKind = "revoked_subscription"
	churnIssuePendingPayment      churnIssueKind = "pending_payment"
)

type churnIssueRecord struct {
	userID             int64
	displayName        string
	primaryAccount     string
	fullName           string
	email              string
	phone              string
	connectorID        int64
	connector          string
	issueType          churnIssueKind
	autoPayEnabled     bool
	recurringState     recurringPaymentState
	paymentStatus      domain.PaymentStatus
	subscriptionID     int64
	subscriptionStatus domain.SubscriptionStatus
	lastAmountRUB      int64
	lastEventAt        time.Time
}

func (h *Handler) churnPage(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)
	data := churnPageData{
		basePageData: basePageData{
			Lang:       lang,
			I18N:       dictForLang(lang),
			CSRFToken:  h.ensureCSRFToken(w, r),
			TopbarPath: "/admin/churn",
			ActiveNav:  "churn",
		},
		UserID:      strings.TrimSpace(r.URL.Query().Get("user_id")),
		TelegramID:  strings.TrimSpace(r.URL.Query().Get("telegram_id")),
		ConnectorID: strings.TrimSpace(r.URL.Query().Get("connector_id")),
		Search:      strings.TrimSpace(r.URL.Query().Get("search")),
		IssueType:   strings.TrimSpace(r.URL.Query().Get("issue_type")),
		AutoPay:     strings.TrimSpace(r.URL.Query().Get("autopay")),
		RetryState:  strings.TrimSpace(r.URL.Query().Get("retry_state")),
		ExportURL:   buildExportURL("/admin/churn/export.csv", r.URL.Query(), lang),
	}

	userFilterID, err := h.resolveFilterUserID(r.Context(), r.URL.Query().Get("user_id"), data.TelegramID)
	if err != nil {
		data.Notice = err.Error()
		h.renderer.render(w, "churn.html", data)
		return
	}

	issues, err := h.buildChurnIssues(r.Context(), lang, userFilterID, data.TelegramID, data.ConnectorID, data.Search, data.IssueType, data.AutoPay, data.RetryState)
	if err != nil {
		data.Notice = err.Error()
		h.renderer.render(w, "churn.html", data)
		return
	}
	data.Issues = issues
	h.renderer.render(w, "churn.html", data)
}

func (h *Handler) buildChurnIssues(ctx context.Context, lang string, userFilterID int64, telegramIDRaw, connectorIDRaw, searchRaw, issueTypeRaw, autoPayRaw, retryStateRaw string) ([]churnIssueView, error) {
	users, err := h.store.ListUsers(ctx, domain.UserListQuery{Limit: 5000})
	if err != nil {
		return nil, err
	}
	payments, err := h.store.ListPayments(ctx, domain.PaymentListQuery{Limit: 5000})
	if err != nil {
		return nil, err
	}
	subs, err := h.store.ListSubscriptions(ctx, domain.SubscriptionListQuery{Limit: 5000})
	if err != nil {
		return nil, err
	}
	connectorNames := h.loadConnectorNames(ctx)
	resolveAccountPresentation := h.buildMessengerAccountPresentationLookup(ctx, lang)

	userMap := make(map[int64]domain.UserListItem, len(users))
	for _, user := range users {
		userMap[user.UserID] = user
	}

	type key struct {
		userID      int64
		connectorID int64
	}
	latestPayment := make(map[key]domain.Payment)
	for _, payment := range payments {
		k := key{userID: payment.UserID, connectorID: payment.ConnectorID}
		prev, ok := latestPayment[k]
		if !ok || payment.CreatedAt.After(prev.CreatedAt) || (payment.CreatedAt.Equal(prev.CreatedAt) && payment.ID > prev.ID) {
			latestPayment[k] = payment
		}
	}

	latestSub := make(map[key]domain.Subscription)
	for _, sub := range subs {
		k := key{userID: sub.UserID, connectorID: sub.ConnectorID}
		prev, ok := latestSub[k]
		if !ok || sub.UpdatedAt.After(prev.UpdatedAt) || (sub.UpdatedAt.Equal(prev.UpdatedAt) && sub.ID > prev.ID) {
			latestSub[k] = sub
		}
	}

	telegramFilter := parseInt64Default(telegramIDRaw)
	connectorFilter := parseInt64Default(connectorIDRaw)
	search := strings.ToLower(strings.TrimSpace(searchRaw))
	issueFilter := churnIssueKind(strings.TrimSpace(issueTypeRaw))
	autoPayFilter := strings.TrimSpace(autoPayRaw)
	retryFilter := recurringRetryFilter(strings.TrimSpace(retryStateRaw))

	allKeys := make(map[key]struct{}, len(latestPayment)+len(latestSub))
	for k := range latestPayment {
		allKeys[k] = struct{}{}
	}
	for k := range latestSub {
		allKeys[k] = struct{}{}
	}

	records := make([]churnIssueRecord, 0, len(allKeys))
	for k := range allKeys {
		if userFilterID > 0 && k.userID != userFilterID {
			continue
		}
		accountPresentation, err := resolveAccountPresentation(k.userID)
		if err != nil {
			return nil, err
		}
		resolvedTelegramID, _ := resolveTelegramIdentityFromUserListItem(userMap[k.userID])
		if telegramFilter > 0 && resolvedTelegramID != telegramFilter {
			continue
		}
		if connectorFilter > 0 && k.connectorID != connectorFilter {
			continue
		}

		payment, hasPayment := latestPayment[k]
		sub, hasSub := latestSub[k]

		var issue churnIssueKind
		switch {
		case hasSub && sub.Status == domain.SubscriptionStatusRevoked:
			issue = churnIssueRevokedSubscription
		case hasSub && sub.Status == domain.SubscriptionStatusExpired:
			issue = churnIssueExpiredSubscription
		case hasPayment && payment.Status == domain.PaymentStatusFailed:
			issue = churnIssueFailedPayment
		case hasPayment && payment.Status == domain.PaymentStatusPending:
			issue = churnIssuePendingPayment
		default:
			continue
		}
		if issueFilter != "" && issue != issueFilter {
			continue
		}

		user := userMap[k.userID]
		connector := connectorDisplayName(connectorNames, k.connectorID)
		autoPayEnabled := hasSub && sub.AutoPayEnabled
		hasAutoPaySettings := hasSub
		recurringState := buildRecurringPaymentState(payments, sub.ID)
		if !matchesAutoPayFilter(autoPayFilter, autoPayEnabled, hasAutoPaySettings) {
			continue
		}
		if !matchesRecurringRetryFilter(retryFilter, autoPayEnabled, recurringState) {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				strconv.FormatInt(resolvedTelegramID, 10),
				accountPresentation.DisplayName,
				accountPresentation.PrimaryAccount,
				user.FullName,
				user.Email,
				user.Phone,
				connector,
			}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}

		lastAt := time.Time{}
		if hasPayment {
			lastAt = payment.CreatedAt
		}
		if hasSub && (lastAt.IsZero() || sub.UpdatedAt.After(lastAt)) {
			lastAt = sub.UpdatedAt
		}

		records = append(records, churnIssueRecord{
			userID:             user.UserID,
			displayName:        coalesceUserDisplayName(user.FullName, accountPresentation.DisplayName, user.UserID),
			primaryAccount:     accountPresentation.PrimaryAccount,
			fullName:           user.FullName,
			email:              user.Email,
			phone:              user.Phone,
			connectorID:        k.connectorID,
			connector:          connector,
			issueType:          issue,
			autoPayEnabled:     autoPayEnabled,
			recurringState:     recurringState,
			paymentStatus:      payment.Status,
			subscriptionID:     sub.ID,
			subscriptionStatus: sub.Status,
			lastAmountRUB:      payment.AmountRUB,
			lastEventAt:        lastAt,
		})
	}

	sort.Slice(records, func(i, j int) bool {
		if records[i].lastEventAt.Equal(records[j].lastEventAt) {
			if records[i].displayName == records[j].displayName {
				return records[i].connector < records[j].connector
			}
			return records[i].displayName < records[j].displayName
		}
		return records[i].lastEventAt.After(records[j].lastEventAt)
	})

	result := make([]churnIssueView, 0, len(records))
	for _, item := range records {
		paymentLabel, paymentClass := paymentStatusBadge(lang, item.paymentStatus)
		subLabel, subClass := subscriptionStatusBadge(lang, item.subscriptionStatus)
		issueLabel, issueClass := localizeChurnIssue(lang, item.issueType)
		autoPayLabel, autoPayClass := autoPayBadge(lang, item.autoPayEnabled, item.subscriptionID > 0)
		retryLabel, retryClass := recurringRetryBadge(lang, item.autoPayEnabled, item.recurringState)
		lastRetryAt := "—"
		if !item.recurringState.LastAttemptAt.IsZero() {
			lastRetryAt = item.recurringState.LastAttemptAt.In(time.Local).Format("2006-01-02 15:04:05")
		}
		result = append(result, churnIssueView{
			UserID:             item.userID,
			DisplayName:        item.displayName,
			PrimaryAccount:     item.primaryAccount,
			FullName:           item.fullName,
			Email:              item.email,
			Phone:              item.phone,
			ConnectorID:        item.connectorID,
			Connector:          item.connector,
			IssueType:          string(item.issueType),
			IssueLabel:         issueLabel,
			IssueClass:         issueClass,
			AutoPayLabel:       autoPayLabel,
			AutoPayClass:       autoPayClass,
			RetryLabel:         retryLabel,
			RetryClass:         retryClass,
			LastRetryAt:        lastRetryAt,
			PaymentStatus:      string(item.paymentStatus),
			PaymentLabel:       paymentLabel,
			PaymentClass:       paymentClass,
			SubscriptionID:     item.subscriptionID,
			SubscriptionStatus: string(item.subscriptionStatus),
			SubscriptionLabel:  subLabel,
			SubscriptionClass:  subClass,
			LastAmountRUB:      item.lastAmountRUB,
			LastEventAt:        item.lastEventAt.In(time.Local).Format("2006-01-02 15:04:05"),
			UserDetailURL:      buildUserDetailURL(lang, item.userID),
			CanSendPayLink:     buildAdminBotStartURL(h.botUsername, h.lookupStartPayload(ctx, item.connectorID)) != "",
			PaymentLinkURL:     buildConnectorPaymentLinkURL(lang, item.userID, item.connectorID),
			CanTriggerRebill:   h.retriggerRebill != nil && item.subscriptionID > 0 && item.autoPayEnabled && item.subscriptionStatus == domain.SubscriptionStatusActive,
			RebillURL:          buildSubscriptionRebillURL(lang, item.userID, item.subscriptionID),
		})
	}

	return result, nil
}

func (h *Handler) lookupStartPayload(ctx context.Context, connectorID int64) string {
	connector, found, err := h.store.GetConnector(ctx, connectorID)
	if err != nil || !found {
		return ""
	}
	return connector.StartPayload
}

func localizeChurnIssue(lang string, issue churnIssueKind) (string, string) {
	switch issue {
	case churnIssueFailedPayment:
		return t(lang, "churn.issue.failed_payment"), "is-danger"
	case churnIssueExpiredSubscription:
		return t(lang, "churn.issue.expired_subscription"), "is-warning"
	case churnIssueRevokedSubscription:
		return t(lang, "churn.issue.revoked_subscription"), "is-danger"
	case churnIssuePendingPayment:
		return t(lang, "churn.issue.pending_payment"), "is-muted"
	default:
		return string(issue), "is-muted"
	}
}

func buildConnectorPaymentLinkURL(lang string, userID, connectorID int64) string {
	params := url.Values{}
	params.Set("lang", lang)
	params.Set("user_id", strconv.FormatInt(userID, 10))
	params.Set("connector_id", strconv.FormatInt(connectorID, 10))
	return "/admin/users/send-payment-link?" + params.Encode()
}

func resolveTelegramIdentityFromUserListItem(user domain.UserListItem) (int64, string) {
	return user.TelegramID, user.TelegramUsername
}
