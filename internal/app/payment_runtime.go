package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	apppayments "github.com/Jopoleon/invest-control-bot/internal/app/payments"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/payment"
	"github.com/Jopoleon/invest-control-bot/internal/store"
)

type mockCheckoutPageData struct {
	Token       string
	ConnectorID int64
	Amount      string
	SuccessURL  string
}

type mockPaymentSuccessPageData struct {
	Token string
}

type paymentRuntime struct {
	store                    store.Store
	robokassaService         *payment.RobokassaService
	adminToken               string
	telegramBotUsername      string
	maxBotUsername           string
	sendUserNotificationFn   func(context.Context, int64, string, messenger.OutgoingMessage) error
	buildAppTargetAuditEvent func(context.Context, int64, string, int64, string, string, time.Time) domain.AuditEvent
	buildTelegramAccessLink  func(context.Context, int64, domain.Connector) (string, error)
	resolvePreferredKindFn   func(context.Context, int64, string) messenger.Kind
	triggerRebillFn          func(context.Context, int64, string) (rebillResponse, error)
}

func (a *application) payments() *paymentRuntime {
	return &paymentRuntime{
		store:                    a.store,
		robokassaService:         a.robokassaService,
		adminToken:               a.config.Security.AdminToken,
		telegramBotUsername:      a.config.Telegram.BotUsername,
		maxBotUsername:           a.config.MAX.BotUsername,
		sendUserNotificationFn:   a.sendUserNotification,
		buildAppTargetAuditEvent: a.buildAppTargetAuditEvent,
		buildTelegramAccessLink:  a.buildTelegramPaymentAccessLink,
		resolvePreferredKindFn:   a.resolvePreferredMessengerKind,
		triggerRebillFn:          a.triggerRebill,
	}
}

// businessService narrows payment-specific side effects behind the payment
// service package so HTTP handlers in the root app package do not have to own
// subscription activation and recurring-failure notification rules directly.
func (p *paymentRuntime) businessService() *apppayments.Service {
	return &apppayments.Service{
		Store:                   p.store,
		TelegramBotUsername:     p.telegramBotUsername,
		SuccessChannelHint:      appPaymentSuccessChannelHint,
		OpenChannelActionLabel:  appPaymentActionOpenChannel,
		MySubscriptionAction:    appPaymentActionMySubscription,
		FailedRecurringText:     appPaymentFailedRecurringText,
		FailedRecurringButton:   appPaymentFailedRecurringButton,
		PaymentSuccessMessage:   appPaymentSuccessMessage,
		BuildTelegramAccessLink: p.buildTelegramAccessLink,
		ResolvePreferredKind:    p.resolvePreferredKindFn,
		SendUserNotification:    p.sendUserNotificationFn,
		BuildTargetAuditEvent:   p.buildAppTargetAuditEvent,
	}
}

func (p *paymentRuntime) handlePaymentResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if p.robokassaService == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	outSum := firstNonEmpty(r.FormValue("OutSum"), r.FormValue("out_summ"))
	invID := firstNonEmpty(r.FormValue("InvId"), r.FormValue("InvID"), r.FormValue("InvoiceID"), r.FormValue("invoice_id"))
	signature := firstNonEmpty(r.FormValue("SignatureValue"), r.FormValue("signaturevalue"))
	if outSum == "" || invID == "" || signature == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if !p.robokassaService.VerifyResultSignature(outSum, invID, signature) {
		logWarn("robokassa result signature mismatch", "inv_id", invID)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	paymentRow, ok, err := p.store.GetPaymentByToken(r.Context(), invID)
	if err != nil {
		logStoreError("load payment by robokassa inv_id failed", err, "inv_id", invID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !ok {
		logWarn("payment not found for robokassa inv_id", "inv_id", invID)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	amountKopeks, parseErr := parseRobokassaAmountToKopeks(outSum)
	if parseErr != nil {
		logWarn("robokassa outsum parse failed", "inv_id", invID, "out_sum", outSum, "error", parseErr)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	expectedKopeks := paymentRow.AmountRUB * 100
	if amountKopeks != expectedKopeks {
		logWarn("robokassa outsum mismatch", "inv_id", invID, "expected_kopeks", expectedKopeks, "actual_kopeks", amountKopeks)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	p.activateSuccessfulPayment(r.Context(), paymentRow, "robokassa:"+invID, now)
	if err := p.store.SaveAuditEvent(r.Context(), p.buildAppTargetAuditEvent(
		r.Context(),
		paymentRow.UserID,
		"",
		paymentRow.ConnectorID,
		domain.AuditActionRobokassaResultReceived,
		"inv_id="+invID+";out_sum="+outSum,
		now,
	)); err != nil {
		logAuditError(domain.AuditActionRobokassaResultReceived, err)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK" + invID))
}

func (p *paymentRuntime) handlePaymentSuccess(w http.ResponseWriter, r *http.Request) {
	if p.robokassaService == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	outSum := firstNonEmpty(r.FormValue("OutSum"), r.FormValue("out_summ"))
	invID := firstNonEmpty(r.FormValue("InvId"), r.FormValue("InvID"), r.FormValue("InvoiceID"), r.FormValue("invoice_id"))
	signature := firstNonEmpty(r.FormValue("SignatureValue"), r.FormValue("signaturevalue"))
	if outSum != "" && invID != "" && signature != "" && !p.robokassaService.VerifySuccessSignature(outSum, invID, signature) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	botURL := buildBotChatURL(p.telegramBotUsername)
	channelURL := ""
	var paymentRow domain.Payment
	var paymentFound bool
	var connector domain.Connector
	var connectorFound bool
	if invID != "" {
		if loadedPayment, ok, err := p.store.GetPaymentByToken(r.Context(), invID); err == nil && ok {
			paymentRow = loadedPayment
			paymentFound = true
			if loadedConnector, found, err := p.store.GetConnector(r.Context(), paymentRow.ConnectorID); err == nil && found {
				connector = loadedConnector
				connectorFound = true
				channelURL = connector.AccessURL(messengerKindToDomain(p.resolvePreferredKindFn(r.Context(), paymentRow.UserID, "")))
			}
		}
	}
	actions := []paymentPageAction{
		{Label: appPaymentActionOpenBot, URL: botURL},
		{Label: appPaymentActionOpenChannel, URL: channelURL, Secondary: true},
		{Label: appPaymentActionOpenTelegram, URL: "https://t.me"},
	}
	if paymentFound {
		actions = p.buildPaymentPageActions(r.Context(), paymentRow, channelURL, true)
	}
	renderPaymentPage(w, paymentPageData{
		Title:   appPaymentPageTitleSuccess,
		Message: appPaymentPageMessageSuccess,
		Details: func() []paymentPageDetail {
			if !paymentFound {
				return nil
			}
			return p.buildPaymentSuccessDetails(r.Context(), paymentRow, connector, connectorFound)
		}(),
		Actions: actions,
	})
}

func (p *paymentRuntime) handlePaymentFail(w http.ResponseWriter, r *http.Request) {
	if p.robokassaService == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	_ = r.ParseForm()
	invID := firstNonEmpty(
		r.FormValue("InvId"),
		r.FormValue("InvID"),
		r.FormValue("InvoiceID"),
		r.FormValue("invoice_id"),
		r.URL.Query().Get("InvId"),
		r.URL.Query().Get("InvID"),
		r.URL.Query().Get("InvoiceID"),
		r.URL.Query().Get("invoice_id"),
	)
	if invID != "" {
		if paymentRow, ok, err := p.store.GetPaymentByToken(r.Context(), invID); err == nil && ok {
			if updated, err := p.store.UpdatePaymentFailed(r.Context(), paymentRow.ID, "robokassa:"+invID, time.Now().UTC()); err != nil {
				logStoreError("mark payment failed failed", err, "payment_id", paymentRow.ID)
			} else if updated {
				_ = p.store.SaveAuditEvent(r.Context(), p.buildAppTargetAuditEvent(
					r.Context(),
					paymentRow.UserID,
					"",
					paymentRow.ConnectorID,
					domain.AuditActionPaymentFailed,
					"payment_id="+strconv.FormatInt(paymentRow.ID, 10),
					time.Now().UTC(),
				))
				p.notifyFailedRecurringPayment(r.Context(), paymentRow)
			}
		}
	}
	actions := []paymentPageAction{{Label: appPaymentActionReturnToBot, URL: firstNonEmpty(buildBotChatURL(p.telegramBotUsername), "https://t.me")}}
	if invID != "" {
		if paymentRow, ok, err := p.store.GetPaymentByToken(r.Context(), invID); err == nil && ok {
			channelURL := ""
			if connector, found, err := p.store.GetConnector(r.Context(), paymentRow.ConnectorID); err == nil && found {
				channelURL = connector.AccessURL(messengerKindToDomain(p.resolvePreferredKindFn(r.Context(), paymentRow.UserID, "")))
			}
			actions = p.buildPaymentPageActions(r.Context(), paymentRow, channelURL, false)
		}
	}
	renderPaymentPage(w, paymentPageData{
		Title:   appPaymentPageTitleFail,
		Message: appPaymentPageMessageFail,
		Actions: actions,
	})
}

func (p *paymentRuntime) handlePaymentRebill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if p.robokassaService == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if !authorizedAdminRequest(r, p.adminToken) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	subscriptionID, err := strconv.ParseInt(r.FormValue("subscription_id"), 10, 64)
	if err != nil || subscriptionID <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("subscription_id is required"))
		return
	}
	payload, err := p.triggerRebillFn(r.Context(), subscriptionID, "admin_http")
	if err != nil {
		switch err.Error() {
		case "subscription not found":
			w.WriteHeader(http.StatusNotFound)
		case "subscription is not active", "autopay is disabled for subscription", "parent payment is missing token", "connector not found":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(err.Error()))
		default:
			if errors.Is(err, errRebillRequestFailed) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte("rebill request failed"))
				return
			}
			logStoreError("trigger rebill failed", err, "subscription_id", subscriptionID)
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}
	writeRebillResponse(w, payload)
}

// Mock checkout pages are temporary placeholders until real provider is selected.
func (p *paymentRuntime) handleMockPay(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	token := r.URL.Query().Get("token")
	connectorIDRaw := r.URL.Query().Get("connector_id")
	var connectorID int64
	if parsed, err := strconv.ParseInt(connectorIDRaw, 10, 64); err == nil && parsed > 0 {
		connectorID = parsed
	}
	amount := r.URL.Query().Get("amount_rub")
	paymentRow, found, err := p.store.GetPaymentByToken(r.Context(), token)
	if err != nil {
		logStoreError("load payment by token for mock checkout failed", err, "token", token)
	}
	if found {
		if err := p.store.SaveAuditEvent(r.Context(), p.buildAppTargetAuditEvent(
			r.Context(),
			paymentRow.UserID,
			"",
			connectorID,
			domain.AuditActionMockCheckoutOpened,
			"token="+token,
			time.Now().UTC(),
		)); err != nil {
			logAuditError(domain.AuditActionMockCheckoutOpened, err)
		}
	}
	successURL := "/mock/pay/success?token=" + token + "&connector_id=" + strconv.FormatInt(connectorID, 10)
	renderAppTemplate(w, "mock_pay.html", mockCheckoutPageData{
		Token:       token,
		ConnectorID: connectorID,
		Amount:      amount,
		SuccessURL:  successURL,
	})
}

func (p *paymentRuntime) handleMockPaySuccess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	token := r.URL.Query().Get("token")
	connectorIDRaw := r.URL.Query().Get("connector_id")
	var connectorID int64
	if parsed, err := strconv.ParseInt(connectorIDRaw, 10, 64); err == nil && parsed > 0 {
		connectorID = parsed
	}
	now := time.Now().UTC()
	paymentRow, ok, err := p.store.GetPaymentByToken(r.Context(), token)
	if err != nil {
		logStoreError("load payment by token failed", err, "token", token)
	}
	if ok {
		connectorID = paymentRow.ConnectorID
		p.activateSuccessfulPayment(r.Context(), paymentRow, "mock:"+token, now)
	}
	if ok {
		if err := p.store.SaveAuditEvent(r.Context(), p.buildAppTargetAuditEvent(
			r.Context(),
			paymentRow.UserID,
			"",
			connectorID,
			domain.AuditActionMockPaymentSuccess,
			"token="+token,
			now,
		)); err != nil {
			logAuditError(domain.AuditActionMockPaymentSuccess, err)
		}
	}
	renderAppTemplate(w, "mock_pay_success.html", mockPaymentSuccessPageData{Token: token})
}

func (p *paymentRuntime) buildPaymentPageActions(ctx context.Context, paymentRow domain.Payment, channelURL string, success bool) []paymentPageAction {
	kind := p.resolvePreferredKindFn(ctx, paymentRow.UserID, "")
	botURL := firstNonEmpty(buildBotChatURL(p.telegramBotUsername), "https://t.me")
	if kind == messenger.KindMAX {
		botURL = firstNonEmpty(buildMAXBotChatURL(p.maxBotUsername), "https://web.max.ru/")
		if connector, found, err := p.store.GetConnector(ctx, paymentRow.ConnectorID); err == nil && found {
			botURL = firstNonEmpty(
				buildMAXBotStartURL(p.maxBotUsername, connector.StartPayload),
				botURL,
			)
		}
	}
	return appPaymentPageActions(kind, success, channelURL, botURL)
}

func (p *paymentRuntime) notifyFailedRecurringPayment(ctx context.Context, paymentRow domain.Payment) {
	p.businessService().NotifyFailedRecurringPayment(ctx, paymentRow)
}

func (p *paymentRuntime) activateSuccessfulPayment(ctx context.Context, paymentRow domain.Payment, providerPaymentID string, now time.Time) {
	p.businessService().ActivateSuccessfulPayment(ctx, paymentRow, providerPaymentID, now)
}

type paymentPageAction struct {
	Label     string
	URL       string
	Secondary bool
}

type paymentPageDetail struct {
	Label string
	Value string
}

type paymentPageData struct {
	Title   string
	Message string
	Details []paymentPageDetail
	Actions []paymentPageAction
}

func renderPaymentPage(w http.ResponseWriter, data paymentPageData) {
	renderAppTemplate(w, "payment_status.html", data)
}

func (p *paymentRuntime) buildPaymentSuccessDetails(ctx context.Context, paymentRow domain.Payment, connector domain.Connector, connectorFound bool) []paymentPageDetail {
	details := make([]paymentPageDetail, 0, 8)
	if connectorFound && strings.TrimSpace(connector.Name) != "" {
		details = append(details, paymentPageDetail{Label: "Подписка", Value: strings.TrimSpace(connector.Name)})
	}
	if paymentRow.AmountRUB > 0 {
		details = append(details, paymentPageDetail{Label: "Сумма", Value: fmt.Sprintf("%d ₽", paymentRow.AmountRUB)})
	} else if connectorFound && connector.PriceRUB > 0 {
		details = append(details, paymentPageDetail{Label: "Сумма", Value: fmt.Sprintf("%d ₽", connector.PriceRUB)})
	}
	if connectorFound {
		if periodLabel := strings.TrimSpace(appConnectorPeriodLabel(connector)); periodLabel != "" {
			details = append(details, paymentPageDetail{Label: "Период", Value: periodLabel})
		}
	}
	details = append(details, paymentPageDetail{Label: "Статус платежа", Value: paymentStatusLabel(paymentRow.Status)})
	if paymentRow.PaidAt != nil && !paymentRow.PaidAt.IsZero() {
		details = append(details, paymentPageDetail{Label: "Оплачено", Value: paymentRow.PaidAt.In(time.Local).Format("02.01.2006 15:04")})
	}
	if latestSub, found, err := p.store.GetLatestSubscriptionByUserConnector(ctx, paymentRow.UserID, paymentRow.ConnectorID); err == nil && found && latestSub.PaymentID == paymentRow.ID {
		details = append(details, paymentPageDetail{Label: "Доступ до", Value: latestSub.EndsAt.In(time.Local).Format("02.01.2006 15:04")})
	}
	if paymentRow.ID > 0 {
		details = append(details, paymentPageDetail{Label: "Номер платежа", Value: strconv.FormatInt(paymentRow.ID, 10)})
	}
	if token := strings.TrimSpace(paymentRow.Token); token != "" {
		details = append(details, paymentPageDetail{Label: "InvId", Value: token})
	}
	if providerRef := strings.TrimSpace(paymentRow.ProviderPaymentID); providerRef != "" {
		details = append(details, paymentPageDetail{Label: "Provider reference", Value: providerRef})
	}
	return details
}

func paymentStatusLabel(status domain.PaymentStatus) string {
	switch status {
	case domain.PaymentStatusPaid:
		return "Оплачен"
	case domain.PaymentStatusFailed:
		return "Не прошел"
	case domain.PaymentStatusPending:
		return "Ожидает подтверждения"
	default:
		if strings.TrimSpace(string(status)) == "" {
			return "Неизвестно"
		}
		return string(status)
	}
}

type rebillResponse struct {
	OK        bool   `json:"ok"`
	InvoiceID string `json:"invoice_id"`
	Existing  bool   `json:"existing,omitempty"`
}

func writeRebillResponse(w http.ResponseWriter, payload rebillResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(payload)
}
