package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/recurringlink"
)

const recurringCancelPageTokenTTL = 180 * 24 * time.Hour

type recurringCheckoutPageData struct {
	Title             string
	ConnectorName     string
	ConnectorDesc     string
	PriceRUB          int64
	PeriodDays        int
	ChannelURL        string
	OfferURL          string
	PrivacyURL        string
	AgreementURL      string
	BotStartURL       string
	HasRecurringDocs  bool
	RecurringDisabled bool
	HelperNote        string
}

type recurringCancelPageData struct {
	Title               string
	Token               string
	TelegramID          int64
	UserName            string
	AutoPayEnabled      bool
	SuccessMessage      string
	ErrorMessage        string
	ExpiresAt           string
	ActiveSubscriptions []recurringCancelSubscriptionView
	OtherSubscriptions  int
}

type recurringCancelSubscriptionView struct {
	SubscriptionID int64
	Name           string
	PriceRUB       int64
	PeriodDays     int
	EndsAtLabel    string
	ChannelURL     string
}

func (a *application) handleRecurringCheckout(w http.ResponseWriter, r *http.Request) {
	payload := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/subscribe/"))
	if payload == "" {
		http.NotFound(w, r)
		return
	}
	connector, ok, err := a.lookupConnectorByPayload(r.Context(), payload)
	if err != nil {
		logStoreError("lookup connector for recurring page failed", err, "payload", payload)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok || !connector.IsActive {
		http.NotFound(w, r)
		return
	}

	offerURL := strings.TrimSpace(connector.OfferURL)
	if offerURL == "" {
		offerURL = a.resolvePublicLegalURL(r.Context(), domain.LegalDocumentTypeOffer)
	}
	privacyURL := strings.TrimSpace(connector.PrivacyURL)
	if privacyURL == "" {
		privacyURL = a.resolvePublicLegalURL(r.Context(), domain.LegalDocumentTypePrivacy)
	}
	agreementURL := a.resolvePublicLegalURL(r.Context(), domain.LegalDocumentTypeUserAgreement)

	renderAppTemplate(w, "recurring_checkout.html", recurringCheckoutPageData{
		Title:             "Оформление подписки",
		ConnectorName:     connector.Name,
		ConnectorDesc:     strings.TrimSpace(connector.Description),
		PriceRUB:          connector.PriceRUB,
		PeriodDays:        connector.PeriodDays,
		ChannelURL:        resolveConnectorChannelURL(connector.ChannelURL, connector.ChatID),
		OfferURL:          offerURL,
		PrivacyURL:        privacyURL,
		AgreementURL:      agreementURL,
		BotStartURL:       buildBotStartURL(a.config.Telegram.BotUsername, connector.StartPayload),
		HasRecurringDocs:  offerURL != "" && agreementURL != "",
		RecurringDisabled: !a.config.Payment.Robokassa.RecurringEnabled,
		HelperNote:        "Автоплатеж подключается во время новой оплаты этого тарифа. Уже действующая подписка не переводится на автосписания задним числом.",
	})
}

func (a *application) handleRecurringCancel(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/unsubscribe/"))
	if token == "" {
		http.NotFound(w, r)
		return
	}
	telegramID, expiresAt, err := recurringlink.ParseCancelToken(a.config.Security.EncryptionKey, token, time.Now().UTC())
	if err != nil {
		status := http.StatusBadRequest
		message := "Некорректная ссылка отключения автоплатежа."
		if errors.Is(err, recurringlink.ErrExpiredToken) {
			status = http.StatusGone
			message = "Ссылка отключения автоплатежа истекла. Откройте новую ссылку из бота."
		}
		w.WriteHeader(status)
		renderAppTemplate(w, "recurring_cancel.html", recurringCancelPageData{
			Title:        "Отключение автоплатежа",
			ErrorMessage: message,
		})
		return
	}

	if r.Method == http.MethodPost {
		a.processRecurringCancelPost(w, r, token, telegramID)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pageData, statusCode := a.buildRecurringCancelPageData(recurringCancelContext(r), token, telegramID, expiresAt)
	w.WriteHeader(statusCode)
	renderAppTemplate(w, "recurring_cancel.html", pageData)
}

func (a *application) processRecurringCancelPost(w http.ResponseWriter, r *http.Request, token string, telegramID int64) {
	now := time.Now().UTC()
	if err := r.ParseForm(); err != nil {
		pageData, _ := a.buildRecurringCancelPageData(r.Context(), token, telegramID, now.Add(recurringCancelPageTokenTTL))
		pageData.ErrorMessage = "Некорректный запрос отключения автоплатежа."
		renderAppTemplate(w, "recurring_cancel.html", pageData)
		return
	}
	subscriptionID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("subscription_id")), 10, 64)
	if err != nil || subscriptionID <= 0 {
		pageData, _ := a.buildRecurringCancelPageData(r.Context(), token, telegramID, now.Add(recurringCancelPageTokenTTL))
		pageData.ErrorMessage = "Не выбрана подписка для отключения автоплатежа."
		renderAppTemplate(w, "recurring_cancel.html", pageData)
		return
	}
	sub, found, err := a.store.GetSubscriptionByID(r.Context(), subscriptionID)
	if err != nil || !found || sub.TelegramID != telegramID {
		pageData, _ := a.buildRecurringCancelPageData(r.Context(), token, telegramID, now.Add(recurringCancelPageTokenTTL))
		pageData.ErrorMessage = "Подписка для отключения автоплатежа не найдена."
		renderAppTemplate(w, "recurring_cancel.html", pageData)
		return
	}
	if sub.Status != domain.SubscriptionStatusActive || !sub.AutoPayEnabled {
		pageData, _ := a.buildRecurringCancelPageData(r.Context(), token, telegramID, now.Add(recurringCancelPageTokenTTL))
		pageData.ErrorMessage = "Для этой подписки автоплатеж уже выключен."
		renderAppTemplate(w, "recurring_cancel.html", pageData)
		return
	}
	if err := a.store.SetSubscriptionAutoPayEnabled(r.Context(), sub.ID, false, now); err != nil {
		logStoreError("disable subscription autopay via public page failed", err, "telegram_id", telegramID, "subscription_id", sub.ID)
		pageData, _ := a.buildRecurringCancelPageData(r.Context(), token, telegramID, now.Add(recurringCancelPageTokenTTL))
		pageData.ErrorMessage = "Не удалось отключить автоплатеж для выбранной подписки. Попробуйте еще раз позже."
		renderAppTemplate(w, "recurring_cancel.html", pageData)
		return
	}
	a.syncUserAutoPayPreferenceAfterPublicCancel(r.Context(), telegramID, now)
	connectorName := ""
	if connector, found, err := a.store.GetConnector(r.Context(), sub.ConnectorID); err == nil && found {
		connectorName = connector.Name
	}
	if err := a.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
		TelegramID:  telegramID,
		ConnectorID: sub.ConnectorID,
		Action:      domain.AuditActionAutopayDisabled,
		Details:     "source=web_cancel_page;subscription_id=" + strconv.FormatInt(sub.ID, 10),
		CreatedAt:   now,
	}); err != nil {
		logAuditError(domain.AuditActionAutopayDisabled, err)
	}
	message := "🔁 Автоплатеж отключен через страницу управления подпиской. Новые автоматические списания больше не будут выполняться, а доступ сохранится до конца уже оплаченного периода."
	if strings.TrimSpace(connectorName) != "" {
		message = "🔁 Автоплатеж для подписки \"" + connectorName + "\" отключен через страницу управления. Новые автоматические списания для этого тарифа больше не будут выполняться, а доступ сохранится до конца уже оплаченного периода."
	}
	if err := a.telegramClient.SendMessage(r.Context(), telegramID, message, nil); err != nil {
		slog.Error("send public cancel confirmation failed", "error", err, "telegram_id", telegramID)
	}
	done := "1"
	if connectorName != "" {
		done = url.QueryEscape(connectorName)
	}
	http.Redirect(w, r, "/unsubscribe/"+url.PathEscape(token)+"?done="+done, http.StatusSeeOther)
}

func (a *application) buildRecurringCancelPageData(ctx context.Context, token string, telegramID int64, expiresAt time.Time) (recurringCancelPageData, int) {
	data := recurringCancelPageData{
		Title:      "Отключение автоплатежа",
		Token:      token,
		TelegramID: telegramID,
		ExpiresAt:  expiresAt.In(time.Local).Format("02.01.2006 15:04"),
	}
	if done, _ := ctx.Value(recurringCancelDoneContextKey{}).(string); strings.TrimSpace(done) != "" {
		data.SuccessMessage = "Автоплатеж отключен. Уже оплаченный период сохранится до конца срока подписки."
		if done != "1" {
			data.SuccessMessage = "Автоплатеж для подписки \"" + done + "\" отключен. Уже оплаченный период сохранится до конца срока подписки."
		}
	}
	user, found, err := a.resolveTelegramUser(ctx, telegramID)
	if err != nil {
		logStoreError("load user for public cancel page failed", err, "telegram_id", telegramID)
		data.ErrorMessage = "Не удалось загрузить данные подписки."
		return data, http.StatusInternalServerError
	}
	if found {
		data.UserName = firstNonEmpty(strings.TrimSpace(user.FullName), strings.TrimSpace(user.TelegramUsername))
	}
	enabled, _, err := a.store.GetUserAutoPayEnabled(ctx, telegramID)
	if err != nil {
		logStoreError("load autopay flag for public cancel page failed", err, "telegram_id", telegramID)
		data.ErrorMessage = "Не удалось загрузить статус автоплатежа."
		return data, http.StatusInternalServerError
	}
	data.AutoPayEnabled = enabled
	subs, err := a.store.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		TelegramID: telegramID,
		Status:     domain.SubscriptionStatusActive,
		Limit:      20,
	})
	if err != nil {
		logStoreError("list subscriptions for public cancel page failed", err, "telegram_id", telegramID)
		data.ErrorMessage = "Не удалось загрузить подписки."
		return data, http.StatusInternalServerError
	}
	data.ActiveSubscriptions = make([]recurringCancelSubscriptionView, 0, len(subs))
	for _, sub := range subs {
		connector, ok, err := a.store.GetConnector(ctx, sub.ConnectorID)
		if err != nil || !ok {
			continue
		}
		if !sub.AutoPayEnabled {
			data.OtherSubscriptions++
			continue
		}
		data.ActiveSubscriptions = append(data.ActiveSubscriptions, recurringCancelSubscriptionView{
			SubscriptionID: sub.ID,
			Name:           connector.Name,
			PriceRUB:       connector.PriceRUB,
			PeriodDays:     connector.PeriodDays,
			EndsAtLabel:    sub.EndsAt.In(time.Local).Format("02.01.2006 15:04"),
			ChannelURL:     resolveConnectorChannelURL(connector.ChannelURL, connector.ChatID),
		})
	}
	data.AutoPayEnabled = len(data.ActiveSubscriptions) > 0
	return data, http.StatusOK
}

type recurringCancelDoneContextKey struct{}

func (a *application) lookupConnectorByPayload(ctx context.Context, payload string) (domain.Connector, bool, error) {
	connector, ok, err := a.store.GetConnectorByStartPayload(ctx, payload)
	if err != nil {
		return domain.Connector{}, false, err
	}
	if ok {
		return connector, true, nil
	}
	if id, parseErr := strconv.ParseInt(payload, 10, 64); parseErr == nil && id > 0 {
		return a.store.GetConnector(ctx, id)
	}
	return domain.Connector{}, false, nil
}

func (a *application) resolvePublicLegalURL(ctx context.Context, docType domain.LegalDocumentType) string {
	doc, found, err := a.store.GetActiveLegalDocument(ctx, docType)
	if err != nil || !found {
		return ""
	}
	if explicit := strings.TrimSpace(doc.ExternalURL); explicit != "" {
		return explicit
	}
	baseURL := strings.TrimRight(publicBaseURL(a.config.Telegram.Webhook.PublicURL), "/")
	if baseURL == "" || doc.ID <= 0 {
		return ""
	}
	switch docType {
	case domain.LegalDocumentTypeOffer:
		return baseURL + "/oferta/" + strconv.FormatInt(doc.ID, 10)
	case domain.LegalDocumentTypePrivacy:
		return baseURL + "/policy/" + strconv.FormatInt(doc.ID, 10)
	case domain.LegalDocumentTypeUserAgreement:
		return baseURL + "/agreement/" + strconv.FormatInt(doc.ID, 10)
	default:
		return ""
	}
}

func recurringCancelContext(r *http.Request) context.Context {
	return context.WithValue(r.Context(), recurringCancelDoneContextKey{}, strings.TrimSpace(r.URL.Query().Get("done")))
}

func (a *application) syncUserAutoPayPreferenceAfterPublicCancel(ctx context.Context, telegramID int64, now time.Time) {
	subs, err := a.store.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		TelegramID: telegramID,
		Status:     domain.SubscriptionStatusActive,
		Limit:      20,
	})
	if err != nil {
		logStoreError("list subscriptions for user autopay sync failed", err, "telegram_id", telegramID)
		return
	}
	enabled := false
	for _, sub := range subs {
		if sub.AutoPayEnabled {
			enabled = true
			break
		}
	}
	if err := a.store.SetUserAutoPayEnabled(ctx, telegramID, enabled, now); err != nil {
		logStoreError("sync user autopay after public cancel failed", err, "telegram_id", telegramID)
	}
}
