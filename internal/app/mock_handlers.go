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
	amount := r.URL.Query().Get("amount_rub")
	paymentRow, found, err := a.store.GetPaymentByToken(r.Context(), token)
	if err != nil {
		logStoreError("load payment by token for mock checkout failed", err, "token", token)
	}
	if found {
		if err := a.store.SaveAuditEvent(r.Context(), a.buildAppTargetAuditEvent(
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

func (a *application) handleMockPaySuccess(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	token := r.URL.Query().Get("token")
	connectorIDRaw := r.URL.Query().Get("connector_id")
	var connectorID int64
	if parsed, err := strconv.ParseInt(connectorIDRaw, 10, 64); err == nil && parsed > 0 {
		connectorID = parsed
	}
	now := time.Now().UTC()
	paymentRow, ok, err := a.store.GetPaymentByToken(r.Context(), token)
	if err != nil {
		logStoreError("load payment by token failed", err, "token", token)
	}
	if ok {
		connectorID = paymentRow.ConnectorID
		a.activateSuccessfulPayment(r.Context(), paymentRow, "mock:"+token, now)
	}
	if ok {
		if err := a.store.SaveAuditEvent(r.Context(), a.buildAppTargetAuditEvent(
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
