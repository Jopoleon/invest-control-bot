package bot

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/Jopoleon/telega-bot-fedor/internal/payment"
	"github.com/Jopoleon/telega-bot-fedor/internal/store/memory"
	"github.com/Jopoleon/telega-bot-fedor/internal/telegram"
	"github.com/go-telegram/bot/models"
)

func TestHandleCallback_ReusesExistingCompletedProfile(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	h := NewHandler(st, tg, payment.NewMockService("http://localhost:8080"), false, "http://localhost:8080")

	connectorID := seedBotConnector(t, ctx, st, "in-existing-user")
	if err := st.SaveUser(ctx, domain.User{
		TelegramID:       1001,
		TelegramUsername: "existing_user",
		FullName:         "Existing User",
		Phone:            "+79990001122",
		Email:            "existing@example.com",
		UpdatedAt:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save user: %v", err)
	}

	h.handleCallback(ctx, &models.CallbackQuery{
		ID:   "cb-1",
		From: models.User{ID: 1001, Username: "existing_user"},
		Data: "accept_terms:" + int64ToString(connectorID),
	})

	if state, found, err := st.GetRegistrationState(ctx, 1001); err != nil {
		t.Fatalf("get registration state: %v", err)
	} else if found {
		t.Fatalf("unexpected registration state: %+v", state)
	}
}

func TestHandleCallback_RequestsOnlyMissingField(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	h := NewHandler(st, tg, payment.NewMockService("http://localhost:8080"), false, "http://localhost:8080")

	connectorID := seedBotConnector(t, ctx, st, "in-partial-user")
	if err := st.SaveUser(ctx, domain.User{
		TelegramID:       1002,
		TelegramUsername: "partial_user",
		FullName:         "Partial User",
		Email:            "partial@example.com",
		UpdatedAt:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save user: %v", err)
	}

	h.handleCallback(ctx, &models.CallbackQuery{
		ID:   "cb-2",
		From: models.User{ID: 1002, Username: "partial_user"},
		Data: "accept_terms:" + int64ToString(connectorID),
	})

	state, found, err := st.GetRegistrationState(ctx, 1002)
	if err != nil {
		t.Fatalf("get registration state: %v", err)
	}
	if !found {
		t.Fatalf("registration state not found")
	}
	if state.Step != domain.StepPhone {
		t.Fatalf("registration step = %s, want %s", state.Step, domain.StepPhone)
	}
}

func TestHandlePay_DisablesRecurringWhenCapabilityOff(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	robokassa := payment.NewRobokassaService(payment.RobokassaConfig{
		MerchantLogin: "test-merchant",
		Password1:     "test-pass1",
		Password2:     "test-pass2",
		IsTest:        true,
		BaseURL:       "https://auth.robokassa.ru/Merchant/Index.aspx",
	})
	h := NewHandler(st, tg, robokassa, false, "http://localhost:8080")

	connectorID := seedBotConnector(t, ctx, st, "in-pay-no-recurring")
	if err := st.SetUserAutoPayEnabled(ctx, 1003, true, time.Now().UTC()); err != nil {
		t.Fatalf("set user autopay: %v", err)
	}

	h.handlePay(ctx, &models.CallbackQuery{
		ID:   "cb-3",
		From: models.User{ID: 1003},
		Data: "pay:" + int64ToString(connectorID),
	})

	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{TelegramID: 1003, Limit: 10})
	if err != nil {
		t.Fatalf("list payments: %v", err)
	}
	if len(payments) != 1 {
		t.Fatalf("payments len = %d, want 1", len(payments))
	}
	if payments[0].AutoPayEnabled {
		t.Fatalf("payment AutoPayEnabled = true, want false")
	}
	if strings.Contains(payments[0].CheckoutURL, "Recurring=true") {
		t.Fatalf("checkout URL should not contain recurring flag: %s", payments[0].CheckoutURL)
	}
}

func seedBotConnector(t *testing.T, ctx context.Context, st *memory.Store, payload string) int64 {
	t.Helper()

	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload: payload,
		Name:         "test-connector",
		ChatID:       "1003626584986",
		PriceRUB:     4444,
		PeriodDays:   30,
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, payload)
	if err != nil {
		t.Fatalf("get connector by payload: %v", err)
	}
	if !found {
		t.Fatalf("connector not found by payload %s", payload)
	}
	return connector.ID
}

func int64ToString(v int64) string {
	return strconv.FormatInt(v, 10)
}

func TestResolveLegalURL_UsesActiveDocumentFallback(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	h := NewHandler(st, tg, payment.NewMockService("http://localhost:8080"), false, "http://localhost:8080")

	if err := st.CreateLegalDocument(ctx, domain.LegalDocument{
		Type:      domain.LegalDocumentTypeOffer,
		Title:     "Offer v1",
		Content:   "Test offer content",
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create legal document: %v", err)
	}

	got := h.resolveLegalURL(ctx, domain.LegalDocumentTypeOffer)
	want := "http://localhost:8080/legal/offer"
	if got != want {
		t.Fatalf("resolveLegalURL() = %q, want %q", got, want)
	}
}
