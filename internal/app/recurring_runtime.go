package app

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	apprecurring "github.com/Jopoleon/invest-control-bot/internal/app/recurring"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/recurringlink"
	"github.com/Jopoleon/invest-control-bot/internal/store"
)

type recurringCheckoutPageData struct {
	Title             string
	ConnectorName     string
	ConnectorDesc     string
	PriceRUB          int64
	PeriodLabel       string
	ChannelURL        string
	OfferURL          string
	PrivacyURL        string
	AgreementURL      string
	TelegramBotURL    string
	MAXWebURL         string
	StartCommand      string
	PrimaryCTA        string
	MAXTitle          string
	MAXHint           string
	MAXCTA            string
	HasRecurringDocs  bool
	RecurringDisabled bool
	HelperNote        string
	ConsentNote       string
}

type recurringRuntime struct {
	store                    store.Store
	encryptionKey            string
	telegramBotUsername      string
	telegramWebhookPublicURL string
	recurringEnabled         bool
	recurringServiceFn       func() *apprecurring.Service
}

func (a *application) recurring() *recurringRuntime {
	return &recurringRuntime{
		store:                    a.store,
		encryptionKey:            a.config.Security.EncryptionKey,
		telegramBotUsername:      a.config.Telegram.BotUsername,
		telegramWebhookPublicURL: a.config.Telegram.Webhook.PublicURL,
		recurringEnabled:         a.config.Payment.Robokassa.RecurringEnabled,
		recurringServiceFn:       a.recurringService,
	}
}

func (rr *recurringRuntime) handleRecurringCheckout(w http.ResponseWriter, r *http.Request) {
	payload := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/subscribe/"))
	if payload == "" {
		http.NotFound(w, r)
		return
	}
	connector, ok, err := rr.lookupConnectorByPayload(r.Context(), payload)
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
		offerURL = rr.resolvePublicLegalURL(r.Context(), domain.LegalDocumentTypeOffer)
	}
	privacyURL := strings.TrimSpace(connector.PrivacyURL)
	if privacyURL == "" {
		privacyURL = rr.resolvePublicLegalURL(r.Context(), domain.LegalDocumentTypePrivacy)
	}
	agreementURL := rr.resolvePublicLegalURL(r.Context(), domain.LegalDocumentTypeUserAgreement)

	// This page deliberately stays informational. The actual purchase and
	// recurring consent toggle still happen inside the bot, where the messenger
	// identity and current registration/payment state are available.
	renderAppTemplate(w, "recurring_checkout.html", recurringCheckoutPageData{
		Title:             appRecurringCheckoutTitle,
		ConnectorName:     connector.Name,
		ConnectorDesc:     strings.TrimSpace(connector.Description),
		PriceRUB:          connector.PriceRUB,
		PeriodLabel:       appConnectorPeriodLabel(connector),
		ChannelURL:        resolveConnectorChannelURL(connector.ChannelURL, connector.ChatID),
		OfferURL:          offerURL,
		PrivacyURL:        privacyURL,
		AgreementURL:      agreementURL,
		TelegramBotURL:    buildBotStartURL(rr.telegramBotUsername, connector.StartPayload),
		MAXWebURL:         "https://web.max.ru/",
		StartCommand:      buildBotStartCommand(connector.StartPayload),
		PrimaryCTA:        recurringCheckoutPrimaryCTA(rr.telegramBotUsername),
		MAXTitle:          appRecurringCheckoutMAXTitle,
		MAXHint:           appRecurringCheckoutMAXHint,
		MAXCTA:            appRecurringCheckoutMAXCTA,
		HasRecurringDocs:  offerURL != "" && agreementURL != "",
		RecurringDisabled: !rr.recurringEnabled,
		HelperNote:        appRecurringCheckoutHelperNote,
		ConsentNote:       appRecurringCheckoutConsentNote,
	})
}

func (rr *recurringRuntime) handleRecurringCancel(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/unsubscribe/"))
	if token == "" {
		http.NotFound(w, r)
		return
	}
	// Public cancel links are keyed by messenger user id because they are used
	// outside the authenticated bot context and must resolve the internal user
	// only after the signed token is validated.
	messengerUserID, expiresAt, err := recurringlink.ParseCancelToken(rr.encryptionKey, token, time.Now().UTC())
	if err != nil {
		status := http.StatusBadRequest
		message := appRecurringCancelInvalidLink
		if errors.Is(err, recurringlink.ErrExpiredToken) {
			status = http.StatusGone
			message = appRecurringCancelExpiredLink
		}
		w.WriteHeader(status)
		renderAppTemplate(w, "recurring_cancel.html", apprecurring.CancelPageData{
			Title:        appRecurringCancelTitle,
			ErrorMessage: message,
		})
		return
	}

	if r.Method == http.MethodPost {
		rr.processRecurringCancelPost(w, r, token, messengerUserID, expiresAt)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pageData, statusCode := rr.recurringServiceFn().BuildCancelPageData(r.Context(), token, messengerUserID, expiresAt, strings.TrimSpace(r.URL.Query().Get("done")))
	w.WriteHeader(statusCode)
	renderAppTemplate(w, "recurring_cancel.html", pageData)
}

func (rr *recurringRuntime) processRecurringCancelPost(w http.ResponseWriter, r *http.Request, token string, messengerUserID int64, expiresAt time.Time) {
	now := time.Now().UTC()
	if err := r.ParseForm(); err != nil {
		pageData, _ := rr.recurringServiceFn().BuildCancelPageData(r.Context(), token, messengerUserID, expiresAt, "")
		pageData.ErrorMessage = appRecurringCancelInvalidRequest
		renderAppTemplate(w, "recurring_cancel.html", pageData)
		return
	}
	subscriptionID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("subscription_id")), 10, 64)
	if err != nil || subscriptionID <= 0 {
		pageData, _ := rr.recurringServiceFn().BuildCancelPageData(r.Context(), token, messengerUserID, expiresAt, "")
		pageData.ErrorMessage = appRecurringCancelNoSubscription
		renderAppTemplate(w, "recurring_cancel.html", pageData)
		return
	}
	connectorName, pageData, statusCode := rr.recurringServiceFn().ProcessCancelRequest(r.Context(), token, messengerUserID, subscriptionID, expiresAt, now)
	if statusCode != 0 {
		renderAppTemplate(w, "recurring_cancel.html", pageData)
		return
	}
	done := "1"
	if connectorName != "" {
		done = url.QueryEscape(connectorName)
	}
	http.Redirect(w, r, "/unsubscribe/"+url.PathEscape(token)+"?done="+done, http.StatusSeeOther)
}

func (rr *recurringRuntime) lookupConnectorByPayload(ctx context.Context, payload string) (domain.Connector, bool, error) {
	connector, ok, err := rr.store.GetConnectorByStartPayload(ctx, payload)
	if err != nil {
		return domain.Connector{}, false, err
	}
	if ok {
		return connector, true, nil
	}
	if id, parseErr := strconv.ParseInt(payload, 10, 64); parseErr == nil && id > 0 {
		return rr.store.GetConnector(ctx, id)
	}
	return domain.Connector{}, false, nil
}

func (rr *recurringRuntime) resolvePublicLegalURL(ctx context.Context, docType domain.LegalDocumentType) string {
	doc, found, err := rr.store.GetActiveLegalDocument(ctx, docType)
	if err != nil || !found {
		return ""
	}
	if explicit := strings.TrimSpace(doc.ExternalURL); explicit != "" {
		return explicit
	}
	baseURL := strings.TrimRight(publicBaseURL(rr.telegramWebhookPublicURL), "/")
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

func recurringCheckoutPrimaryCTA(botUsername string) string {
	if strings.TrimSpace(strings.TrimPrefix(botUsername, "@")) == "" {
		return appRecurringCheckoutBotCTA
	}
	return appRecurringCheckoutTelegramCTA
}

func appConnectorPeriodLabel(connector domain.Connector) string {
	if fixedEndsAt, ok := connector.FixedDeadline(); ok {
		return "до " + fixedEndsAt.In(time.Local).Format("02.01.2006 15:04")
	}
	if months, ok := connector.CalendarMonthsPeriod(); ok {
		return strconv.Itoa(months) + " мес."
	}
	if duration, ok := connector.DurationPeriod(); ok {
		return formatAppDurationLabel(duration)
	}
	return "30 дн."
}

func formatAppDurationLabel(duration time.Duration) string {
	seconds := int64(duration / time.Second)
	switch {
	case seconds%(24*60*60) == 0:
		return strconv.FormatInt(seconds/(24*60*60), 10) + " дн."
	case seconds%(60*60) == 0:
		return strconv.FormatInt(seconds/(60*60), 10) + " ч."
	case seconds%60 == 0:
		return strconv.FormatInt(seconds/60, 10) + " мин."
	default:
		return strconv.FormatInt(seconds, 10) + " сек."
	}
}
