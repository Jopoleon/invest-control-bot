package app

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/Jopoleon/telega-bot-fedor/internal/payment"
)

type paymentPageAction struct {
	Label     string
	URL       string
	Secondary bool
}

type paymentPageData struct {
	Title   string
	Message string
	Actions []paymentPageAction
}

func (a *application) handlePaymentResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if a.robokassaService == nil {
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
	if !a.robokassaService.VerifyResultSignature(outSum, invID, signature) {
		logWarn("robokassa result signature mismatch", "inv_id", invID)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	paymentRow, ok, err := a.store.GetPaymentByToken(r.Context(), invID)
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
	a.activateSuccessfulPayment(r.Context(), paymentRow, "robokassa:"+invID, time.Now().UTC())
	if err := a.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
		TelegramID:  paymentRow.TelegramID,
		ConnectorID: paymentRow.ConnectorID,
		Action:      "robokassa_result_received",
		Details:     "inv_id=" + invID + ";out_sum=" + outSum,
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		logAuditError("robokassa_result_received", err)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK" + invID))
}

func (a *application) handlePaymentSuccess(w http.ResponseWriter, r *http.Request) {
	if a.robokassaService == nil {
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
	if outSum != "" && invID != "" && signature != "" && !a.robokassaService.VerifySuccessSignature(outSum, invID, signature) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	botURL := buildBotChatURL(a.config.Telegram.BotUsername)
	channelURL := ""
	if invID != "" {
		if paymentRow, ok, err := a.store.GetPaymentByToken(r.Context(), invID); err == nil && ok {
			if connector, found, err := a.store.GetConnector(r.Context(), paymentRow.ConnectorID); err == nil && found {
				channelURL = resolveConnectorChannelURL(connector.ChannelURL, connector.ChatID)
			}
		}
	}
	renderPaymentPage(w, paymentPageData{
		Title:   "Оплата успешно завершена",
		Message: "Платеж подтвержден. Подписка активируется автоматически, а в боте придет сообщение с деталями.",
		Actions: []paymentPageAction{
			{Label: "Открыть бота", URL: botURL},
			{Label: "Открыть канал", URL: channelURL, Secondary: true},
			{Label: "Открыть Telegram", URL: "https://t.me"},
		},
	})
}

func (a *application) handlePaymentFail(w http.ResponseWriter, r *http.Request) {
	if a.robokassaService == nil {
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
		if paymentRow, ok, err := a.store.GetPaymentByToken(r.Context(), invID); err == nil && ok {
			if updated, err := a.store.UpdatePaymentFailed(r.Context(), paymentRow.ID, "robokassa:"+invID, time.Now().UTC()); err != nil {
				logStoreError("mark payment failed failed", err, "payment_id", paymentRow.ID)
			} else if updated {
				_ = a.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
					TelegramID:  paymentRow.TelegramID,
					ConnectorID: paymentRow.ConnectorID,
					Action:      "payment_failed",
					Details:     "payment_id=" + strconv.FormatInt(paymentRow.ID, 10),
					CreatedAt:   time.Now().UTC(),
				})
			}
		}
	}
	botURL := buildBotChatURL(a.config.Telegram.BotUsername)
	if botURL == "" {
		botURL = "https://t.me"
	}
	renderPaymentPage(w, paymentPageData{
		Title:   "Оплата не завершена",
		Message: "Платеж был отменен или не прошел. Вернитесь в бота и попробуйте снова.",
		Actions: []paymentPageAction{{Label: "Вернуться в бота", URL: botURL}},
	})
}

func (a *application) handlePaymentRebill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if a.robokassaService == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if !authorizedAdminRequest(r, a.config.Security.AdminToken) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	subscriptionID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("subscription_id")), 10, 64)
	if err != nil || subscriptionID <= 0 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("subscription_id is required"))
		return
	}

	subscription, found, err := a.store.GetSubscriptionByID(r.Context(), subscriptionID)
	if err != nil {
		logStoreError("get subscription for rebill failed", err, "subscription_id", subscriptionID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !found {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if subscription.Status != domain.SubscriptionStatusActive {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("subscription is not active"))
		return
	}
	if !subscription.AutoPayEnabled {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("autopay is disabled for subscription"))
		return
	}
	if pending, ok, err := a.store.GetPendingRebillPaymentBySubscription(r.Context(), subscription.ID); err != nil {
		logStoreError("get pending rebill payment failed", err, "subscription_id", subscription.ID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	} else if ok {
		writeRebillResponse(w, rebillResponse{OK: true, InvoiceID: pending.Token, Existing: true})
		return
	}

	parentPayment, found, err := a.store.GetPaymentByID(r.Context(), subscription.PaymentID)
	if err != nil {
		logStoreError("get parent payment for rebill failed", err, "payment_id", subscription.PaymentID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !found || strings.TrimSpace(parentPayment.Token) == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("parent payment is missing token"))
		return
	}
	connector, found, err := a.store.GetConnector(r.Context(), subscription.ConnectorID)
	if err != nil {
		logStoreError("get connector for rebill failed", err, "connector_id", subscription.ConnectorID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !found {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("connector not found"))
		return
	}

	invoiceID := generateInvoiceID()
	now := time.Now().UTC()
	pendingPayment := domain.Payment{
		Provider:          "robokassa",
		ProviderPaymentID: "rebill_parent:" + parentPayment.Token,
		Status:            domain.PaymentStatusPending,
		Token:             invoiceID,
		TelegramID:        subscription.TelegramID,
		ConnectorID:       subscription.ConnectorID,
		SubscriptionID:    subscription.ID,
		ParentPaymentID:   parentPayment.ID,
		AmountRUB:         connector.PriceRUB,
		AutoPayEnabled:    true,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := a.store.CreatePayment(r.Context(), pendingPayment); err != nil {
		if existing, ok, lookupErr := a.store.GetPendingRebillPaymentBySubscription(r.Context(), subscription.ID); lookupErr == nil && ok {
			writeRebillResponse(w, rebillResponse{OK: true, InvoiceID: existing.Token, Existing: true})
			return
		}
		logStoreError("create rebill payment failed", err, "invoice_id", invoiceID, "subscription_id", subscription.ID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	createdPayment, found, err := a.store.GetPaymentByToken(r.Context(), invoiceID)
	if err != nil {
		logStoreError("load created rebill payment failed", err, "invoice_id", invoiceID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !found {
		logWarn("created rebill payment not found", "invoice_id", invoiceID)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	pendingPayment = createdPayment

	if err := a.robokassaService.CreateRebill(r.Context(), payment.RebillRequest{
		InvoiceID:         invoiceID,
		PreviousInvoiceID: parentPayment.Token,
		AmountRUB:         connector.PriceRUB,
		Description:       connector.Name,
	}); err != nil {
		if _, markErr := a.store.UpdatePaymentFailed(r.Context(), pendingPayment.ID, "rebill_request_failed:"+parentPayment.Token, time.Now().UTC()); markErr != nil {
			logStoreError("mark rebill payment failed failed", markErr, "payment_id", pendingPayment.ID)
		}
		_ = a.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
			TelegramID:  subscription.TelegramID,
			ConnectorID: subscription.ConnectorID,
			Action:      "rebill_request_failed",
			Details:     "subscription_id=" + strconv.FormatInt(subscription.ID, 10) + ";invoice_id=" + invoiceID + ";error=" + err.Error(),
			CreatedAt:   time.Now().UTC(),
		})
		logStoreError("robokassa rebill request failed", err, "subscription_id", subscriptionID, "invoice_id", invoiceID)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("rebill request failed"))
		return
	}
	if err := a.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
		TelegramID:  subscription.TelegramID,
		ConnectorID: subscription.ConnectorID,
		Action:      "rebill_requested",
		Details:     "subscription_id=" + strconv.FormatInt(subscription.ID, 10) + ";invoice_id=" + invoiceID + ";parent=" + parentPayment.Token,
		CreatedAt:   now,
	}); err != nil {
		logAuditError("rebill_requested", err)
	}
	writeRebillResponse(w, rebillResponse{OK: true, InvoiceID: invoiceID})
}

func renderPaymentPage(w http.ResponseWriter, data paymentPageData) {
	renderAppTemplate(w, "payment_status.html", data)
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
