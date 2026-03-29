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
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
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

type recurringCancelPageData struct {
	Title               string
	Token               string
	MessengerUserID     int64
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

	// This page deliberately stays informational. The actual purchase and
	// recurring consent toggle still happen inside the bot, where the messenger
	// identity and current registration/payment state are available.
	renderAppTemplate(w, "recurring_checkout.html", recurringCheckoutPageData{
		Title:             appRecurringCheckoutTitle,
		ConnectorName:     connector.Name,
		ConnectorDesc:     strings.TrimSpace(connector.Description),
		PriceRUB:          connector.PriceRUB,
		PeriodDays:        connector.PeriodDays,
		ChannelURL:        resolveConnectorChannelURL(connector.ChannelURL, connector.ChatID),
		OfferURL:          offerURL,
		PrivacyURL:        privacyURL,
		AgreementURL:      agreementURL,
		TelegramBotURL:    buildBotStartURL(a.config.Telegram.BotUsername, connector.StartPayload),
		MAXWebURL:         "https://web.max.ru/",
		StartCommand:      buildBotStartCommand(connector.StartPayload),
		PrimaryCTA:        recurringCheckoutPrimaryCTA(a.config.Telegram.BotUsername),
		MAXTitle:          appRecurringCheckoutMAXTitle,
		MAXHint:           appRecurringCheckoutMAXHint,
		MAXCTA:            appRecurringCheckoutMAXCTA,
		HasRecurringDocs:  offerURL != "" && agreementURL != "",
		RecurringDisabled: !a.config.Payment.Robokassa.RecurringEnabled,
		HelperNote:        appRecurringCheckoutHelperNote,
		ConsentNote:       appRecurringCheckoutConsentNote,
	})
}

func recurringCheckoutPrimaryCTA(botUsername string) string {
	if strings.TrimSpace(strings.TrimPrefix(botUsername, "@")) == "" {
		return appRecurringCheckoutBotCTA
	}
	return appRecurringCheckoutTelegramCTA
}

func (a *application) handleRecurringCancel(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/unsubscribe/"))
	if token == "" {
		http.NotFound(w, r)
		return
	}
	// Public cancel links are keyed by messenger user id because they are used
	// outside the authenticated bot context and must resolve the internal user
	// only after the signed token is validated.
	messengerUserID, expiresAt, err := recurringlink.ParseCancelToken(a.config.Security.EncryptionKey, token, time.Now().UTC())
	if err != nil {
		status := http.StatusBadRequest
		message := appRecurringCancelInvalidLink
		if errors.Is(err, recurringlink.ErrExpiredToken) {
			status = http.StatusGone
			message = appRecurringCancelExpiredLink
		}
		w.WriteHeader(status)
		renderAppTemplate(w, "recurring_cancel.html", recurringCancelPageData{
			Title:        appRecurringCancelTitle,
			ErrorMessage: message,
		})
		return
	}

	if r.Method == http.MethodPost {
		a.processRecurringCancelPost(w, r, token, messengerUserID)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	pageData, statusCode := a.buildRecurringCancelPageData(recurringCancelContext(r), token, messengerUserID, expiresAt)
	w.WriteHeader(statusCode)
	renderAppTemplate(w, "recurring_cancel.html", pageData)
}

func (a *application) processRecurringCancelPost(w http.ResponseWriter, r *http.Request, token string, messengerUserID int64) {
	now := time.Now().UTC()
	if err := r.ParseForm(); err != nil {
		pageData, _ := a.buildRecurringCancelPageData(r.Context(), token, messengerUserID, now.Add(recurringCancelPageTokenTTL))
		pageData.ErrorMessage = appRecurringCancelInvalidRequest
		renderAppTemplate(w, "recurring_cancel.html", pageData)
		return
	}
	subscriptionID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("subscription_id")), 10, 64)
	if err != nil || subscriptionID <= 0 {
		pageData, _ := a.buildRecurringCancelPageData(r.Context(), token, messengerUserID, now.Add(recurringCancelPageTokenTTL))
		pageData.ErrorMessage = appRecurringCancelNoSubscription
		renderAppTemplate(w, "recurring_cancel.html", pageData)
		return
	}
	sub, found, err := a.store.GetSubscriptionByID(r.Context(), subscriptionID)
	// The page itself is authorized only by the signed token, so every write must
	// re-check that the selected subscription belongs to the same messenger id.
	if err != nil || !found || !a.subscriptionMatchesMessengerUserID(r.Context(), sub, messengerUserID) {
		pageData, _ := a.buildRecurringCancelPageData(r.Context(), token, messengerUserID, now.Add(recurringCancelPageTokenTTL))
		pageData.ErrorMessage = appRecurringCancelMissingSub
		renderAppTemplate(w, "recurring_cancel.html", pageData)
		return
	}
	if sub.Status != domain.SubscriptionStatusActive || !sub.AutoPayEnabled {
		pageData, _ := a.buildRecurringCancelPageData(r.Context(), token, messengerUserID, now.Add(recurringCancelPageTokenTTL))
		pageData.ErrorMessage = appRecurringCancelAlreadyOff
		renderAppTemplate(w, "recurring_cancel.html", pageData)
		return
	}
	if err := a.store.SetSubscriptionAutoPayEnabled(r.Context(), sub.ID, false, now); err != nil {
		logStoreError("disable subscription autopay via public page failed", err, "messenger_user_id", messengerUserID, "subscription_id", sub.ID)
		pageData, _ := a.buildRecurringCancelPageData(r.Context(), token, messengerUserID, now.Add(recurringCancelPageTokenTTL))
		pageData.ErrorMessage = appRecurringCancelPersistFailed
		renderAppTemplate(w, "recurring_cancel.html", pageData)
		return
	}
	connectorName := ""
	if connector, found, err := a.store.GetConnector(r.Context(), sub.ConnectorID); err == nil && found {
		connectorName = connector.Name
	}
	if err := a.store.SaveAuditEvent(r.Context(), a.buildAppTargetAuditEvent(
		r.Context(),
		sub.UserID,
		formatPreferredMessengerUserID(messengerUserID),
		sub.ConnectorID,
		domain.AuditActionAutopayDisabled,
		"source=web_cancel_page;subscription_id="+strconv.FormatInt(sub.ID, 10),
		now,
	)); err != nil {
		logAuditError(domain.AuditActionAutopayDisabled, err)
	}
	message := appRecurringCancelNotification(connectorName)
	// Delivery goes through the linked messenger account for the user. The page
	// is authorized by messenger user id, but the notification itself
	// should follow the real messenger origin of the subscription.
	if err := a.sendUserNotification(r.Context(), sub.UserID, formatPreferredMessengerUserID(messengerUserID), messenger.OutgoingMessage{Text: message}); err != nil {
		slog.Error("send public cancel confirmation failed", "error", err, "user_id", sub.UserID, "preferred_messenger_user_id", messengerUserID)
	}
	done := "1"
	if connectorName != "" {
		done = url.QueryEscape(connectorName)
	}
	http.Redirect(w, r, "/unsubscribe/"+url.PathEscape(token)+"?done="+done, http.StatusSeeOther)
}

func (a *application) subscriptionMatchesMessengerUserID(ctx context.Context, sub domain.Subscription, messengerUserID int64) bool {
	if sub.UserID <= 0 || messengerUserID <= 0 {
		return false
	}
	user, found, err := a.resolveUserByMessengerUserID(ctx, messengerUserID)
	if err == nil && found {
		return user.ID == sub.UserID
	}
	telegramID, found, err := a.resolveTelegramMessengerUserID(ctx, sub.UserID)
	if err != nil || !found {
		return false
	}
	return telegramID == messengerUserID
}

func (a *application) buildRecurringCancelPageData(ctx context.Context, token string, messengerUserID int64, expiresAt time.Time) (recurringCancelPageData, int) {
	data := recurringCancelPageData{
		Title:           appRecurringCancelTitle,
		Token:           token,
		MessengerUserID: messengerUserID,
		ExpiresAt:       expiresAt.In(time.Local).Format("02.01.2006 15:04"),
	}
	if done, _ := ctx.Value(recurringCancelDoneContextKey{}).(string); strings.TrimSpace(done) != "" {
		data.SuccessMessage = appRecurringCancelSuccessForSubscription(done)
	}
	query := domain.SubscriptionListQuery{
		Status: domain.SubscriptionStatusActive,
		Limit:  20,
	}
	if user, found, err := a.resolveUserByMessengerUserID(ctx, messengerUserID); err == nil && found {
		query.UserID = user.ID
	} else if err != nil {
		logStoreError("resolve user for public cancel page failed", err, "messenger_user_id", messengerUserID)
		data.ErrorMessage = appRecurringCancelSubsLoadFail
		return data, http.StatusInternalServerError
	}
	subs, err := a.store.ListSubscriptions(ctx, query)
	if err != nil {
		logStoreError("list subscriptions for public cancel page failed", err, "messenger_user_id", messengerUserID)
		data.ErrorMessage = appRecurringCancelSubsLoadFail
		return data, http.StatusInternalServerError
	}
	// Prefer the canonical user linked from the subscriptions shown on the page.
	// Fall back to the messenger-identity bridge only when the page still needs
	// to resolve a user from the signed token instead of the subscription row.
	if userName := a.resolveRecurringCancelUserName(ctx, subs, messengerUserID); userName != "" {
		data.UserName = userName
	}
	data.ActiveSubscriptions = make([]recurringCancelSubscriptionView, 0, len(subs))
	for _, sub := range subs {
		connector, ok, err := a.store.GetConnector(ctx, sub.ConnectorID)
		if err != nil || !ok {
			continue
		}
		if !sub.AutoPayEnabled {
			// Active subscriptions with already disabled recurring are intentionally
			// not actionable here; they only affect the summary counters.
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
	// The page uses PRG state only for success banners. Subscription state always
	// comes from the store, never from the query string.
	return context.WithValue(r.Context(), recurringCancelDoneContextKey{}, strings.TrimSpace(r.URL.Query().Get("done")))
}

func (a *application) resolveRecurringCancelUserName(ctx context.Context, subs []domain.Subscription, messengerUserID int64) string {
	// Prefer the canonical user record reachable from migrated subscription rows.
	// The messenger lookup remains a read-only bridge for older/public flows that
	// still start from the signed messenger user id instead of an authenticated user.
	for _, sub := range subs {
		if sub.UserID <= 0 {
			continue
		}
		user, found, err := a.store.GetUserByID(ctx, sub.UserID)
		if err != nil {
			logStoreError("load user for public cancel page failed", err, "user_id", sub.UserID, "messenger_user_id", messengerUserID)
			break
		}
		if found {
			if accounts, err := a.store.ListUserMessengerAccounts(ctx, user.ID); err == nil {
				for _, account := range accounts {
					if account.MessengerKind == domain.MessengerKindTelegram {
						return firstNonEmpty(strings.TrimSpace(user.FullName), strings.TrimSpace(account.Username))
					}
				}
			}
			return strings.TrimSpace(user.FullName)
		}
	}
	user, found, err := a.resolveUserByMessengerUserID(ctx, messengerUserID)
	if err != nil {
		logStoreError("load messenger user bridge for public cancel page failed", err, "messenger_user_id", messengerUserID)
		return ""
	}
	if found {
		if accounts, err := a.store.ListUserMessengerAccounts(ctx, user.ID); err == nil {
			for _, account := range accounts {
				if account.MessengerUserID == strconv.FormatInt(messengerUserID, 10) {
					return firstNonEmpty(strings.TrimSpace(user.FullName), strings.TrimSpace(account.Username))
				}
			}
		}
		return strings.TrimSpace(user.FullName)
	}
	return ""
}
