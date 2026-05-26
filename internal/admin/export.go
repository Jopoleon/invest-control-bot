package admin

import (
	"context"
	"encoding/csv"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

const exportLimit = 10000

func (h *Handler) exportConnectorsCSV(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	rows, err := h.store.ListConnectors(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	records := make([][]string, 0, len(rows)+1)
	records = append(records, []string{
		"id", "start_payload", "name", "description", "chat_id", "channel_url", "max_chat_id", "max_channel_url",
		"price_rub", "period_mode", "period_seconds", "period_months", "fixed_ends_at",
		"period_label", "offer_url", "privacy_url", "is_active", "created_at",
	})
	for _, c := range rows {
		fixedEndsAt := ""
		if c.FixedEndsAt != nil {
			fixedEndsAt = c.FixedEndsAt.In(time.Local).Format(time.RFC3339)
		}
		records = append(records, []string{
			strconv.FormatInt(c.ID, 10),
			c.StartPayload,
			c.Name,
			c.Description,
			c.ChatID,
			c.ChannelURL,
			c.MAXChatID,
			c.MAXChannelURL,
			strconv.FormatInt(c.PriceRUB, 10),
			string(c.ResolvedPeriodMode()),
			strconv.FormatInt(c.PeriodSeconds, 10),
			strconv.Itoa(c.PeriodMonths),
			fixedEndsAt,
			adminConnectorPeriodLabel(c),
			c.OfferURL,
			c.PrivacyURL,
			strconv.FormatBool(c.IsActive),
			c.CreatedAt.In(time.Local).Format(time.RFC3339),
		})
	}
	writeCSV(w, exportFilename("connectors"), records)
}

func (h *Handler) exportLegalDocumentsCSV(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	rows, err := h.store.ListLegalDocuments(r.Context(), "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	records := make([][]string, 0, len(rows)+1)
	records = append(records, []string{
		"id", "doc_type", "title", "content", "external_url", "version", "is_active", "created_at",
	})
	for _, doc := range rows {
		records = append(records, []string{
			strconv.FormatInt(doc.ID, 10),
			string(doc.Type),
			doc.Title,
			doc.Content,
			doc.ExternalURL,
			strconv.Itoa(doc.Version),
			strconv.FormatBool(doc.IsActive),
			doc.CreatedAt.In(time.Local).Format(time.RFC3339),
		})
	}
	writeCSV(w, exportFilename("legal_documents"), records)
}

func (h *Handler) exportUsersCSV(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	query := parseUsersQuery(r.URL.Query())
	if userID, err := h.resolveFilterUserID(r.Context(), r.URL.Query().Get("user_id"), r.URL.Query().Get("telegram_id")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if userID > 0 {
		query.UserID = userID
	}
	rows, err := h.store.ListUsers(r.Context(), query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resolveAccountPresentation := h.buildMessengerAccountPresentationLookup(r.Context(), h.resolveLang(w, r))
	subs, err := h.store.ListSubscriptions(r.Context(), domain.SubscriptionListQuery{Limit: exportLimit})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	periodCountsByUser := buildUserPeriodCounts(subs, now)
	autopayStatesByUser := buildUserAutopayStates(subs, now)

	records := make([][]string, 0, len(rows)+1)
	records = append(records, []string{
		"user_id", "display_name", "primary_account", "linked_accounts", "full_name", "phone", "email",
		"current_periods", "next_periods", "auto_pay_enabled", "has_auto_pay_settings", "updated_at",
	})
	for _, user := range rows {
		accountPresentation, err := resolveAccountPresentation(user.UserID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		counts := periodCountsByUser[user.UserID]
		autopayState, ok := autopayStatesByUser[user.UserID]
		if !ok {
			autopayState = userAutopayState{
				Enabled:    user.AutoPayEnabled,
				Configured: user.HasAutoPaySettings,
			}
		}
		records = append(records, []string{
			strconv.FormatInt(user.UserID, 10),
			coalesceUserDisplayName(user.FullName, accountPresentation.DisplayName, user.UserID),
			accountPresentation.PrimaryAccount,
			joinAccountDisplays(accountPresentation.Accounts),
			user.FullName,
			user.Phone,
			user.Email,
			strconv.Itoa(counts.Current),
			strconv.Itoa(counts.Next),
			strconv.FormatBool(autopayState.Enabled),
			strconv.FormatBool(autopayState.Configured),
			user.UpdatedAt.In(time.Local).Format(time.RFC3339),
		})
	}
	writeCSV(w, exportFilename("users"), records)
}

func (h *Handler) exportEventsCSV(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	query := parseEventsQuery(r.URL.Query())
	rows, _, err := h.store.ListAuditEvents(r.Context(), query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	connectorNames := h.loadConnectorNames(r.Context())

	records := make([][]string, 0, len(rows)+1)
	records = append(records, []string{
		"id", "created_at", "actor_type", "actor_user_id", "actor_messenger_kind", "actor_messenger_user_id", "actor_subject", "target_user_id", "target_messenger_kind", "target_messenger_user_id", "target_account", "connector_id", "connector", "action", "details",
	})
	for _, event := range rows {
		records = append(records, []string{
			strconv.FormatInt(event.ID, 10),
			event.CreatedAt.In(time.Local).Format(time.RFC3339),
			string(event.ActorType),
			formatOptionalInt64(event.ActorUserID),
			string(event.ActorMessengerKind),
			event.ActorMessengerUserID,
			event.ActorSubject,
			formatOptionalInt64(event.TargetUserID),
			string(event.TargetMessengerKind),
			event.TargetMessengerUserID,
			buildMessengerAccountDisplay(messengerKindLabel(h.resolveLang(w, r), event.TargetMessengerKind), event.TargetMessengerUserID, ""),
			strconv.FormatInt(event.ConnectorID, 10),
			connectorDisplayName(connectorNames, event.ConnectorID),
			event.Action,
			event.Details,
		})
	}
	writeCSV(w, exportFilename("events"), records)
}

func (h *Handler) exportPaymentsCSV(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	paymentQuery, _ := parseBillingQueries(r.URL.Query())
	if userID, err := h.resolveFilterUserID(r.Context(), r.URL.Query().Get("user_id"), r.URL.Query().Get("telegram_id")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if userID > 0 {
		paymentQuery.UserID = userID
	}
	rows, err := h.store.ListPayments(r.Context(), paymentQuery)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	connectorNames, projectNames, monthlyAmounts := h.loadConnectorExportData(r.Context())
	resolveAccountPresentation := h.buildMessengerAccountPresentationLookup(r.Context(), h.resolveLang(w, r))
	resolveExportAccount := h.buildExportMessengerAccountLookup(r.Context())
	paidCounts := h.buildPaidPaymentCounts(r.Context())

	records := make([][]string, 0, len(rows)+1)
	records = append(records, []string{
		"id", "user_id", "primary_account", "provider", "provider_payment_id", "status", "token", "connector_id",
		"connector", "project", "messenger_kind", "messenger_user_id", "messenger_username", "telegram_username",
		"subscription_id", "parent_payment_id", "amount_rub", "monthly_payment_rub", "total_paid_payments", "auto_pay_enabled",
		"checkout_url", "created_at", "paid_at", "updated_at",
	})
	for _, payment := range rows {
		accountPresentation, err := resolveAccountPresentation(payment.UserID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		exportAccount, err := resolveExportAccount(payment.UserID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		paidAt := ""
		if payment.PaidAt != nil {
			paidAt = payment.PaidAt.In(time.Local).Format(time.RFC3339)
		}
		records = append(records, []string{
			strconv.FormatInt(payment.ID, 10),
			strconv.FormatInt(payment.UserID, 10),
			accountPresentation.PrimaryAccount,
			payment.Provider,
			payment.ProviderPaymentID,
			string(payment.Status),
			payment.Token,
			strconv.FormatInt(payment.ConnectorID, 10),
			connectorDisplayName(connectorNames, payment.ConnectorID),
			connectorDisplayName(projectNames, payment.ConnectorID),
			string(exportAccount.MessengerKind),
			exportAccount.MessengerUserID,
			exportAccount.Username,
			exportAccount.TelegramUsername,
			strconv.FormatInt(payment.SubscriptionID, 10),
			strconv.FormatInt(payment.ParentPaymentID, 10),
			strconv.FormatInt(payment.AmountRUB, 10),
			formatMonthlyPaymentRUB(monthlyAmounts, payment.ConnectorID, payment.AmountRUB),
			strconv.Itoa(paidCounts[exportPaymentCountKey(payment.UserID, payment.ConnectorID)]),
			strconv.FormatBool(payment.AutoPayEnabled),
			payment.CheckoutURL,
			payment.CreatedAt.In(time.Local).Format(time.RFC3339),
			paidAt,
			payment.UpdatedAt.In(time.Local).Format(time.RFC3339),
		})
	}
	writeCSV(w, exportFilename("payments"), records)
}

func (h *Handler) exportSubscriptionsCSV(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	_, subQuery := parseBillingQueries(r.URL.Query())
	if userID, err := h.resolveFilterUserID(r.Context(), r.URL.Query().Get("user_id"), r.URL.Query().Get("telegram_id")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else if userID > 0 {
		subQuery.UserID = userID
	}
	rows, err := h.store.ListSubscriptions(r.Context(), subQuery)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	connectorNames, projectNames, monthlyAmounts := h.loadConnectorExportData(r.Context())
	resolveAccountPresentation := h.buildMessengerAccountPresentationLookup(r.Context(), h.resolveLang(w, r))
	resolveExportAccount := h.buildExportMessengerAccountLookup(r.Context())
	paidCounts := h.buildPaidPaymentCounts(r.Context())
	now := time.Now().UTC()
	sortSubscriptionsForOperationalView(rows, now)

	records := make([][]string, 0, len(rows)+1)
	records = append(records, []string{
		"id", "user_id", "primary_account", "connector_id", "connector", "project",
		"messenger_kind", "messenger_user_id", "messenger_username", "telegram_username",
		"payment_id", "status", "phase", "auto_pay_enabled", "monthly_payment_rub", "total_paid_payments",
		"next_payment_at", "reminder_sent_at",
		"expiry_notice_sent_at", "created_at", "updated_at",
	})
	for _, sub := range rows {
		accountPresentation, err := resolveAccountPresentation(sub.UserID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		exportAccount, err := resolveExportAccount(sub.UserID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		records = append(records, []string{
			strconv.FormatInt(sub.ID, 10),
			strconv.FormatInt(sub.UserID, 10),
			accountPresentation.PrimaryAccount,
			strconv.FormatInt(sub.ConnectorID, 10),
			connectorDisplayName(connectorNames, sub.ConnectorID),
			connectorDisplayName(projectNames, sub.ConnectorID),
			string(exportAccount.MessengerKind),
			exportAccount.MessengerUserID,
			exportAccount.Username,
			exportAccount.TelegramUsername,
			strconv.FormatInt(sub.PaymentID, 10),
			string(sub.Status),
			subscriptionPhaseLabel(sub, now),
			strconv.FormatBool(sub.AutoPayEnabled),
			formatMonthlyPaymentRUB(monthlyAmounts, sub.ConnectorID, 0),
			strconv.Itoa(paidCounts[exportPaymentCountKey(sub.UserID, sub.ConnectorID)]),
			formatNextPaymentAt(sub, now),
			formatOptionalTime(sub.ReminderSentAt),
			formatOptionalTime(sub.ExpiryNoticeSentAt),
			sub.CreatedAt.In(time.Local).Format(time.RFC3339),
			sub.UpdatedAt.In(time.Local).Format(time.RFC3339),
		})
	}
	writeCSV(w, exportFilename("subscriptions"), records)
}

func (h *Handler) exportChurnCSV(w http.ResponseWriter, r *http.Request) {
	if !h.requireAuth(w, r) {
		return
	}
	lang := h.resolveLang(w, r)
	telegramIDRaw := strings.TrimSpace(r.URL.Query().Get("telegram_id"))
	userFilterID, err := h.resolveFilterUserID(r.Context(), r.URL.Query().Get("user_id"), telegramIDRaw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rows, err := h.buildChurnIssues(
		r.Context(),
		lang,
		userFilterID,
		telegramIDRaw,
		strings.TrimSpace(r.URL.Query().Get("connector_id")),
		strings.TrimSpace(r.URL.Query().Get("search")),
		strings.TrimSpace(r.URL.Query().Get("issue_type")),
		strings.TrimSpace(r.URL.Query().Get("autopay")),
		strings.TrimSpace(r.URL.Query().Get("retry_state")),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	records := make([][]string, 0, len(rows)+1)
	records = append(records, []string{
		"user_id", "display_name", "primary_account", "full_name", "email", "phone",
		"connector_id", "connector", "issue_type", "autopay", "retry_state", "last_retry_at", "payment_status",
		"subscription_id", "subscription_status", "last_amount_rub", "last_event_at",
	})
	for _, row := range rows {
		records = append(records, []string{
			strconv.FormatInt(row.UserID, 10),
			row.DisplayName,
			row.PrimaryAccount,
			row.FullName,
			row.Email,
			row.Phone,
			strconv.FormatInt(row.ConnectorID, 10),
			row.Connector,
			row.IssueType,
			row.AutoPayLabel,
			row.RetryLabel,
			row.LastRetryAt,
			row.PaymentStatus,
			strconv.FormatInt(row.SubscriptionID, 10),
			row.SubscriptionStatus,
			strconv.FormatInt(row.LastAmountRUB, 10),
			row.LastEventAt,
		})
	}
	writeCSV(w, exportFilename("churn"), records)
}

func parseUsersQuery(params url.Values) domain.UserListQuery {
	query := domain.UserListQuery{
		Limit:  exportLimit,
		Search: strings.TrimSpace(params.Get("search")),
	}
	if id, err := strconv.ParseInt(strings.TrimSpace(params.Get("user_id")), 10, 64); err == nil && id > 0 {
		query.UserID = id
	}
	if id, err := strconv.ParseInt(strings.TrimSpace(params.Get("telegram_id")), 10, 64); err == nil && id > 0 {
		query.TelegramID = id
	}
	return query
}

func parseBillingQueries(params url.Values) (domain.PaymentListQuery, domain.SubscriptionListQuery) {
	paymentQuery := domain.PaymentListQuery{Limit: exportLimit}
	subQuery := domain.SubscriptionListQuery{Limit: exportLimit}

	if id, err := strconv.ParseInt(strings.TrimSpace(params.Get("user_id")), 10, 64); err == nil && id > 0 {
		paymentQuery.UserID = id
		subQuery.UserID = id
	}
	if id, err := strconv.ParseInt(strings.TrimSpace(params.Get("connector_id")), 10, 64); err == nil && id > 0 {
		paymentQuery.ConnectorID = id
		subQuery.ConnectorID = id
	}
	if status := strings.TrimSpace(params.Get("payment_status")); status != "" {
		paymentQuery.Status = domain.PaymentStatus(status)
	}
	if status := strings.TrimSpace(params.Get("subscription_status")); status != "" {
		subQuery.Status = domain.SubscriptionStatus(status)
	}
	if from, ok := parseDateAtLocalStart(strings.TrimSpace(params.Get("date_from"))); ok {
		paymentQuery.CreatedFrom = &from
		subQuery.CreatedFrom = &from
	}
	if to, ok := parseDateAtLocalEndExclusive(strings.TrimSpace(params.Get("date_to"))); ok {
		paymentQuery.CreatedToExclude = &to
		subQuery.CreatedToExclude = &to
	}

	return paymentQuery, subQuery
}

func parseEventsQuery(params url.Values) domain.AuditEventListQuery {
	query := domain.AuditEventListQuery{
		SortBy:   "created_at",
		SortDesc: true,
		Page:     1,
		PageSize: exportLimit,
		Action:   strings.TrimSpace(params.Get("action")),
		Search:   strings.TrimSpace(params.Get("search")),
	}

	rawActorType := strings.TrimSpace(params.Get("actor_type"))
	if rawActorType == string(domain.AuditActorTypeAdmin) || rawActorType == string(domain.AuditActorTypeUser) || rawActorType == string(domain.AuditActorTypeApp) {
		query.ActorType = domain.AuditActorType(rawActorType)
	}
	rawMessengerKind := strings.TrimSpace(params.Get("messenger_kind"))
	if rawMessengerKind == string(domain.MessengerKindTelegram) || rawMessengerKind == string(domain.MessengerKindMAX) {
		query.TargetMessengerKind = domain.MessengerKind(rawMessengerKind)
	}
	query.TargetMessengerUserID = strings.TrimSpace(params.Get("messenger_user_id"))
	if id, err := strconv.ParseInt(strings.TrimSpace(params.Get("connector_id")), 10, 64); err == nil && id > 0 {
		query.ConnectorID = id
	}
	if from, ok := parseDateAtLocalStart(strings.TrimSpace(params.Get("date_from"))); ok {
		query.CreatedFrom = &from
	}
	if to, ok := parseDateAtLocalEndExclusive(strings.TrimSpace(params.Get("date_to"))); ok {
		query.CreatedToExclude = &to
	}

	switch sortBy := strings.TrimSpace(params.Get("sort_by")); sortBy {
	case "actor_type", "target_messenger_user_id", "connector_id", "action", "created_at":
		query.SortBy = sortBy
	}
	if strings.EqualFold(strings.TrimSpace(params.Get("sort_dir")), "asc") {
		query.SortDesc = false
	}
	return query
}

type exportMessengerAccount struct {
	MessengerKind    domain.MessengerKind
	MessengerUserID  string
	Username         string
	TelegramUsername string
}

func (h *Handler) buildExportMessengerAccountLookup(ctx context.Context) func(int64) (exportMessengerAccount, error) {
	cache := make(map[int64]exportMessengerAccount)
	return func(userID int64) (exportMessengerAccount, error) {
		if cached, ok := cache[userID]; ok {
			return cached, nil
		}
		accounts, err := h.store.ListUserMessengerAccounts(ctx, userID)
		if err != nil {
			return exportMessengerAccount{}, err
		}
		preferred, _ := pickPreferredMessengerAccount(accounts)
		item := exportMessengerAccount{
			MessengerKind:   preferred.MessengerKind,
			MessengerUserID: strings.TrimSpace(preferred.MessengerUserID),
			Username:        strings.TrimPrefix(strings.TrimSpace(preferred.Username), "@"),
		}
		for _, account := range accounts {
			if account.MessengerKind == domain.MessengerKindTelegram {
				item.TelegramUsername = strings.TrimPrefix(strings.TrimSpace(account.Username), "@")
				break
			}
		}
		cache[userID] = item
		return item, nil
	}
}

func (h *Handler) buildPaidPaymentCounts(ctx context.Context) map[string]int {
	payments, err := h.store.ListPayments(ctx, domain.PaymentListQuery{Status: domain.PaymentStatusPaid, Limit: exportLimit})
	if err != nil {
		return map[string]int{}
	}
	counts := make(map[string]int)
	for _, payment := range payments {
		counts[exportPaymentCountKey(payment.UserID, payment.ConnectorID)]++
	}
	return counts
}

func exportPaymentCountKey(userID, connectorID int64) string {
	return strconv.FormatInt(userID, 10) + ":" + strconv.FormatInt(connectorID, 10)
}

func formatNextPaymentAt(sub domain.Subscription, now time.Time) string {
	if !sub.IsCurrentActiveAt(now) || !sub.AutoPayEnabled {
		return ""
	}
	return sub.EndsAt.In(time.Local).Format(time.RFC3339)
}

func formatMonthlyPaymentRUB(amounts map[int64]int64, connectorID, fallbackAmount int64) string {
	amount := amounts[connectorID]
	if amount <= 0 {
		amount = fallbackAmount
	}
	if amount <= 0 {
		return ""
	}
	return strconv.FormatInt(amount, 10)
}

func (h *Handler) loadConnectorExportData(ctx context.Context) (map[int64]string, map[int64]string, map[int64]int64) {
	connectors, err := h.store.ListConnectors(ctx)
	if err != nil {
		return map[int64]string{}, map[int64]string{}, map[int64]int64{}
	}
	telegramProjects := h.loadTelegramProjectNames(ctx)
	connectorNames := make(map[int64]string, len(connectors))
	projectNames := make(map[int64]string, len(connectors))
	monthlyAmounts := make(map[int64]int64, len(connectors))
	for _, connector := range connectors {
		connectorNames[connector.ID] = connector.Name
		projectNames[connector.ID] = connectorProjectName(connector, telegramProjects)
		monthlyAmounts[connector.ID] = connector.PriceRUB
	}
	return connectorNames, projectNames, monthlyAmounts
}

func (h *Handler) loadTelegramProjectNames(ctx context.Context) map[string]string {
	chats, err := h.store.ListTelegramChats(ctx)
	if err != nil {
		return map[string]string{}
	}
	names := make(map[string]string, len(chats)*3)
	for _, chat := range chats {
		title := telegramProjectTitle(chat)
		if title == "" {
			continue
		}
		for _, key := range telegramChatProjectKeys(chat) {
			names[key] = title
		}
	}
	return names
}

func telegramProjectTitle(chat domain.TelegramChat) string {
	if title := strings.TrimSpace(chat.Title); title != "" {
		return title
	}
	if username := strings.TrimSpace(chat.Username); username != "" {
		return "@" + strings.TrimPrefix(username, "@")
	}
	return strings.TrimSpace(chat.ChatID)
}

func telegramChatProjectKeys(chat domain.TelegramChat) []string {
	rawChatID := strings.TrimSpace(chat.ChatID)
	username := strings.TrimSpace(chat.Username)
	keys := make([]string, 0, 4)
	if rawChatID != "" {
		keys = append(keys, rawChatID, strings.TrimPrefix(rawChatID, "-"))
	}
	if username != "" {
		keys = append(keys, "@"+strings.TrimPrefix(username, "@"), strings.TrimPrefix(username, "@"))
	}
	return keys
}

func connectorProjectName(connector domain.Connector, telegramProjects map[string]string) string {
	if ref := connector.ResolvedTelegramChatRef(); ref != "" {
		if name := strings.TrimSpace(telegramProjects[ref]); name != "" {
			return name
		}
		if name := strings.TrimSpace(telegramProjects[strings.TrimPrefix(ref, "-")]); name != "" {
			return name
		}
	}
	return firstNonEmptyExportValue(
		strings.TrimSpace(connector.Name),
		strings.TrimSpace(connector.ChannelURL),
		strings.TrimSpace(connector.MAXChannelURL),
		strings.TrimSpace(connector.MAXChatID),
		strings.TrimSpace(connector.ChatID),
	)
}

func firstNonEmptyExportValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func buildExportURL(path string, params url.Values, lang string) string {
	values := cloneURLValues(params)
	values.Del("page")
	values.Set("lang", lang)
	return path + "?" + values.Encode()
}

func exportFilename(prefix string) string {
	return prefix + "_" + time.Now().Format("20060102_150405") + ".csv"
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.In(time.Local).Format(time.RFC3339)
}

func joinAccountDisplays(accounts []messengerAccountView) string {
	if len(accounts) == 0 {
		return ""
	}
	values := make([]string, 0, len(accounts))
	for _, account := range accounts {
		if strings.TrimSpace(account.Display) == "" {
			continue
		}
		values = append(values, account.Display)
	}
	return strings.Join(values, " | ")
}

func writeCSV(w http.ResponseWriter, filename string, records [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	writer := csv.NewWriter(w)
	_ = writer.Write([]string{"sep=,"})
	_ = writer.WriteAll(records)
	writer.Flush()
}
