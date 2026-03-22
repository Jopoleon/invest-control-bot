package app

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

type mockCheckoutPageData struct {
	Token       string
	ConnectorID int64
	UserID      string
	Amount      string
	SuccessURL  string
}

type mockPaymentSuccessPageData struct {
	Token string
}

// Mock checkout pages are temporary placeholders until real provider is selected.
func (a *application) handleMockPay(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	token := r.URL.Query().Get("token")
	connectorIDRaw := r.URL.Query().Get("connector_id")
	var connectorID int64
	if parsed, err := strconv.ParseInt(connectorIDRaw, 10, 64); err == nil && parsed > 0 {
		connectorID = parsed
	}
	userID := r.URL.Query().Get("user_id")
	amount := r.URL.Query().Get("amount_rub")
	if tgID, err := strconv.ParseInt(userID, 10, 64); err == nil && tgID > 0 {
		if err := a.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
			TelegramID:  tgID,
			ConnectorID: connectorID,
			Action:      domain.AuditActionMockCheckoutOpened,
			Details:     "token=" + token,
			CreatedAt:   time.Now().UTC(),
		}); err != nil {
			logAuditError(domain.AuditActionMockCheckoutOpened, err)
		}
	}
	successURL := "/mock/pay/success?token=" + token + "&connector_id=" + strconv.FormatInt(connectorID, 10) + "&user_id=" + userID
	renderAppTemplate(w, "mock_pay.html", mockCheckoutPageData{
		Token:       token,
		ConnectorID: connectorID,
		UserID:      userID,
		Amount:      amount,
		SuccessURL:  successURL,
	})
}

func (a *application) handleMockPaySuccess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	token := r.URL.Query().Get("token")
	connectorIDRaw := r.URL.Query().Get("connector_id")
	var connectorID int64
	if parsed, err := strconv.ParseInt(connectorIDRaw, 10, 64); err == nil && parsed > 0 {
		connectorID = parsed
	}
	userID := r.URL.Query().Get("user_id")
	now := time.Now().UTC()
	paymentRow, ok, err := a.store.GetPaymentByToken(r.Context(), token)
	if err != nil {
		logStoreError("load payment by token failed", err, "token", token)
	}
	if ok {
		connectorID = paymentRow.ConnectorID
		userID = strconv.FormatInt(paymentRow.TelegramID, 10)
		a.activateSuccessfulPayment(r.Context(), paymentRow, "mock:"+token, now)
	}
	if tgID, err := strconv.ParseInt(userID, 10, 64); err == nil && tgID > 0 {
		if err := a.store.SaveAuditEvent(r.Context(), domain.AuditEvent{
			TelegramID:  tgID,
			ConnectorID: connectorID,
			Action:      domain.AuditActionMockPaymentSuccess,
			Details:     "token=" + token,
			CreatedAt:   now,
		}); err != nil {
			logAuditError(domain.AuditActionMockPaymentSuccess, err)
		}
	}
	renderAppTemplate(w, "mock_pay_success.html", mockPaymentSuccessPageData{Token: token})
}
