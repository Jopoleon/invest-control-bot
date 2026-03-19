package app

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/config"
	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/Jopoleon/telega-bot-fedor/internal/store"
	"github.com/Jopoleon/telega-bot-fedor/internal/store/memory"
)

func TestPaymentResult_SuccessAndIdempotency(t *testing.T) {
	t.Helper()

	const (
		pass2 = "test-pass2"
		invID = "1000000001"
	)

	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-result-success")
	seedPayment(t, ctx, st, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       invID,
		TelegramID:  777001,
		ConnectorID: connectorID,
		AmountRUB:   2322,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	})

	handler := testServerHandler(t, st, pass2)

	outSum := "2322.00"
	signature := resultSignature(outSum, invID, pass2)

	firstCode, firstBody := postPaymentResult(t, handler, outSum, invID, signature)
	if firstCode != http.StatusOK {
		t.Fatalf("first callback status = %d, want %d (body=%q)", firstCode, http.StatusOK, firstBody)
	}
	if firstBody != "OK"+invID {
		t.Fatalf("first callback body = %q, want %q", firstBody, "OK"+invID)
	}

	secondCode, secondBody := postPaymentResult(t, handler, outSum, invID, signature)
	if secondCode != http.StatusOK {
		t.Fatalf("second callback status = %d, want %d (body=%q)", secondCode, http.StatusOK, secondBody)
	}
	if secondBody != "OK"+invID {
		t.Fatalf("second callback body = %q, want %q", secondBody, "OK"+invID)
	}

	paymentRow, found, err := st.GetPaymentByToken(ctx, invID)
	if err != nil {
		t.Fatalf("get payment by token: %v", err)
	}
	if !found {
		t.Fatalf("payment not found by token=%s", invID)
	}
	if paymentRow.Status != domain.PaymentStatusPaid {
		t.Fatalf("payment status = %s, want %s", paymentRow.Status, domain.PaymentStatusPaid)
	}

	subs, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		TelegramID: paymentRow.TelegramID,
		Status:     domain.SubscriptionStatusActive,
		Limit:      20,
	})
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("active subscriptions = %d, want 1", len(subs))
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 200})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}

	if got := countAuditEvents(events, "payment_success_notified"); got != 1 {
		t.Fatalf("payment_success_notified count = %d, want 1", got)
	}
	if got := countAuditEvents(events, "robokassa_result_received"); got != 2 {
		t.Fatalf("robokassa_result_received count = %d, want 2", got)
	}
}

func TestPaymentResult_RejectsOutSumMismatch(t *testing.T) {
	t.Helper()

	const (
		pass2 = "test-pass2"
		invID = "1000000002"
	)

	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-result-mismatch")
	seedPayment(t, ctx, st, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       invID,
		TelegramID:  777002,
		ConnectorID: connectorID,
		AmountRUB:   2322,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	})

	handler := testServerHandler(t, st, pass2)

	outSum := "1.00"
	signature := resultSignature(outSum, invID, pass2)
	code, _ := postPaymentResult(t, handler, outSum, invID, signature)
	if code != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want %d", code, http.StatusBadRequest)
	}

	paymentRow, found, err := st.GetPaymentByToken(ctx, invID)
	if err != nil {
		t.Fatalf("get payment by token: %v", err)
	}
	if !found {
		t.Fatalf("payment not found by token=%s", invID)
	}
	if paymentRow.Status != domain.PaymentStatusPending {
		t.Fatalf("payment status = %s, want %s", paymentRow.Status, domain.PaymentStatusPending)
	}

	subs, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		TelegramID: paymentRow.TelegramID,
		Limit:      20,
	})
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("subscriptions = %d, want 0", len(subs))
	}
}

func TestPaymentRebill_CreatesPendingRecurringPayment(t *testing.T) {
	t.Helper()

	const (
		adminToken = "admin-secret"
		pass2      = "test-pass2"
		parentInv  = "1000009001"
	)

	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-rebill")

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          parentInv,
		TelegramID:     778001,
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	})
	parentPayment, found, err := st.GetPaymentByToken(ctx, parentInv)
	if err != nil || !found {
		t.Fatalf("parent payment not found: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		TelegramID:     parentPayment.TelegramID,
		ConnectorID:    parentPayment.ConnectorID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       time.Now().UTC().Add(-24 * time.Hour),
		EndsAt:         time.Now().UTC().Add(24 * time.Hour),
		CreatedAt:      time.Now().UTC().Add(-24 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	subscriptions, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		TelegramID: parentPayment.TelegramID,
		Limit:      20,
	})
	if err != nil || len(subscriptions) != 1 {
		t.Fatalf("expected one subscription, got=%d err=%v", len(subscriptions), err)
	}

	rebillMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.FormValue("PreviousInvoiceID") != parentInv {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("bad parent"))
			return
		}
		_, _ = w.Write([]byte("OK+" + r.FormValue("InvoiceID")))
	}))
	defer rebillMock.Close()

	cfg := config.Config{
		AppName:     "telega-bot-fedor-test",
		Environment: config.EnvLocal,
		HTTP: config.HTTPConfig{
			Address:      ":0",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
		Postgres: config.PostgresConfig{
			Driver: "memory",
		},
		Payment: config.PaymentConfig{
			Provider: "robokassa",
			Robokassa: config.RobokassaPaymentConfig{
				MerchantLogin: "test-merchant",
				Password1:     "test-pass1",
				Password2:     pass2,
				IsTestMode:    true,
				RebillURL:     rebillMock.URL,
			},
		},
		Security: config.SecurityConfig{
			AdminToken: adminToken,
		},
	}
	handler := testServerHandlerWithConfig(t, st, cfg)

	form := url.Values{"subscription_id": {fmt.Sprintf("%d", subscriptions[0].ID)}}
	req := httptest.NewRequest(http.MethodPost, "/payment/rebill", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		body, _ := io.ReadAll(rr.Body)
		t.Fatalf("rebill status=%d body=%q", rr.Code, string(body))
	}
	var payload struct {
		OK        bool   `json:"ok"`
		InvoiceID string `json:"invoice_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode rebill response: %v", err)
	}
	if !payload.OK || strings.TrimSpace(payload.InvoiceID) == "" {
		t.Fatalf("unexpected rebill payload: %+v", payload)
	}

	newPayment, found, err := st.GetPaymentByToken(ctx, payload.InvoiceID)
	if err != nil || !found {
		t.Fatalf("new rebill payment not found: found=%v err=%v", found, err)
	}
	if newPayment.Status != domain.PaymentStatusPending {
		t.Fatalf("new payment status=%s want=%s", newPayment.Status, domain.PaymentStatusPending)
	}
	if !newPayment.AutoPayEnabled {
		t.Fatalf("new payment autopay should be enabled")
	}
}

func testServerHandler(t *testing.T, st store.Store, pass2 string) http.Handler {
	t.Helper()

	cfg := config.Config{
		AppName:     "telega-bot-fedor-test",
		Environment: config.EnvLocal,
		HTTP: config.HTTPConfig{
			Address:      ":0",
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		},
		Postgres: config.PostgresConfig{
			Driver: "memory",
		},
		Payment: config.PaymentConfig{
			Provider: "robokassa",
			Robokassa: config.RobokassaPaymentConfig{
				MerchantLogin: "test-merchant",
				Password1:     "test-pass1",
				Password2:     pass2,
				IsTestMode:    true,
			},
		},
	}

	return testServerHandlerWithConfig(t, st, cfg)
}

func testServerHandlerWithConfig(t *testing.T, st store.Store, cfg config.Config) http.Handler {
	t.Helper()
	srv, err := New(cfg, st)
	if err != nil {
		t.Fatalf("create app server: %v", err)
	}
	return srv.httpServer.Handler
}

func seedConnector(t *testing.T, ctx context.Context, st store.Store, payload string) int64 {
	t.Helper()

	err := st.CreateConnector(ctx, domain.Connector{
		StartPayload: payload,
		Name:         "test-connector",
		ChatID:       "1003626584986",
		PriceRUB:     2322,
		PeriodDays:   30,
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create connector: %v", err)
	}

	connector, found, err := st.GetConnectorByStartPayload(ctx, payload)
	if err != nil {
		t.Fatalf("get connector by payload: %v", err)
	}
	if !found {
		t.Fatalf("connector not found by payload=%s", payload)
	}
	return connector.ID
}

func seedPayment(t *testing.T, ctx context.Context, st store.Store, payment domain.Payment) {
	t.Helper()
	if err := st.CreatePayment(ctx, payment); err != nil {
		t.Fatalf("create payment: %v", err)
	}
}

func postPaymentResult(t *testing.T, handler http.Handler, outSum, invID, signature string) (int, string) {
	t.Helper()

	form := url.Values{
		"OutSum":         {outSum},
		"InvId":          {invID},
		"SignatureValue": {signature},
	}
	req := httptest.NewRequest(http.MethodPost, "/payment/result", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body, err := io.ReadAll(rr.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return rr.Code, strings.TrimSpace(string(body))
}

func resultSignature(outSum, invID, pass2 string) string {
	return md5Hex(fmt.Sprintf("%s:%s:%s", strings.TrimSpace(outSum), strings.TrimSpace(invID), strings.TrimSpace(pass2)))
}

func md5Hex(raw string) string {
	sum := md5.Sum([]byte(raw))
	return fmt.Sprintf("%x", sum[:])
}

func countAuditEvents(events []domain.AuditEvent, action string) int {
	count := 0
	for _, event := range events {
		if event.Action == action {
			count++
		}
	}
	return count
}
