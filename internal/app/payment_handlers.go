package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/go-telegram/bot/models"
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
		Action:      domain.AuditActionRobokassaResultReceived,
		Details:     "inv_id=" + invID + ";out_sum=" + outSum,
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		logAuditError(domain.AuditActionRobokassaResultReceived, err)
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
					Action:      domain.AuditActionPaymentFailed,
					Details:     "payment_id=" + strconv.FormatInt(paymentRow.ID, 10),
					CreatedAt:   time.Now().UTC(),
				})
				a.notifyFailedRecurringPayment(r.Context(), paymentRow)
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
	payload, err := a.triggerRebill(r.Context(), subscriptionID, "admin_http")
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

func (a *application) notifyFailedRecurringPayment(ctx context.Context, paymentRow domain.Payment) {
	if !paymentRow.AutoPayEnabled || paymentRow.SubscriptionID <= 0 {
		return
	}
	connector, found, err := a.store.GetConnector(ctx, paymentRow.ConnectorID)
	if err != nil || !found {
		return
	}
	renewURL := buildBotStartURL(a.config.Telegram.BotUsername, connector.StartPayload)
	text := "⚠️ Автоматическое списание не прошло. Чтобы не потерять доступ, оплатите подписку вручную по кнопке ниже."
	var keyboard *models.InlineKeyboardMarkup
	if renewURL != "" {
		keyboard = &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{{
				{Text: "Оплатить вручную", URL: renewURL},
			}},
		}
	}
	if err := a.telegramClient.SendMessage(ctx, paymentRow.TelegramID, text, keyboard); err != nil {
		logWarn("failed recurring payment notify failed", "payment_id", paymentRow.ID, "telegram_id", paymentRow.TelegramID, "error", err)
		return
	}
	if err := a.store.SaveAuditEvent(ctx, domain.AuditEvent{
		TelegramID:  paymentRow.TelegramID,
		ConnectorID: paymentRow.ConnectorID,
		Action:      domain.AuditActionRecurringPaymentFailedNotice,
		Details:     "payment_id=" + strconv.FormatInt(paymentRow.ID, 10),
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		logAuditError(domain.AuditActionRecurringPaymentFailedNotice, err)
	}
}
