package bot

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/payment"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
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
	h := NewHandler(st, tg, payment.NewMockService("http://localhost:8080"), false, "http://localhost:8080", "test-encryption-key-123456789012345")

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
	h := NewHandler(st, tg, payment.NewMockService("http://localhost:8080"), false, "http://localhost:8080", "test-encryption-key-123456789012345")

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

func TestHandleCallback_SavesLegalDocumentVersionsForFallbackDocs(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	h := NewHandler(st, tg, payment.NewMockService("http://localhost:8080"), false, "http://localhost:8080", "test-encryption-key-123456789012345")

	connectorID := seedBotConnector(t, ctx, st, "in-consent-versioned")
	if err := st.CreateLegalDocument(ctx, domain.LegalDocument{
		Type:      domain.LegalDocumentTypeOffer,
		Title:     "Offer v1",
		Content:   "Offer content",
		Version:   1,
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create offer doc: %v", err)
	}
	if err := st.CreateLegalDocument(ctx, domain.LegalDocument{
		Type:      domain.LegalDocumentTypePrivacy,
		Title:     "Privacy v2",
		Content:   "Privacy content",
		Version:   2,
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create privacy doc: %v", err)
	}

	h.handleCallback(ctx, &models.CallbackQuery{
		ID:   "cb-consent-versioned",
		From: models.User{ID: 1101, Username: "versioned_user"},
		Data: "accept_terms:" + int64ToString(connectorID),
	})

	consent, found, err := st.GetConsent(ctx, 1101, connectorID)
	if err != nil {
		t.Fatalf("get consent: %v", err)
	}
	if !found {
		t.Fatalf("consent not found")
	}
	if consent.OfferDocumentID != 1 || consent.OfferDocumentVersion != 1 {
		t.Fatalf("unexpected offer consent versioning: %+v", consent)
	}
	if consent.PrivacyDocumentID != 2 || consent.PrivacyDocumentVersion != 2 {
		t.Fatalf("unexpected privacy consent versioning: %+v", consent)
	}
}

func TestHandleCallback_LeavesConsentVersionsEmptyForConnectorCustomURLs(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	h := NewHandler(st, tg, payment.NewMockService("http://localhost:8080"), false, "http://localhost:8080", "test-encryption-key-123456789012345")

	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload: "in-consent-custom",
		Name:         "custom-consent-connector",
		ChatID:       "1003626584986",
		PriceRUB:     4444,
		PeriodDays:   30,
		OfferURL:     "https://example.com/custom-offer",
		PrivacyURL:   "https://example.com/custom-privacy",
		IsActive:     true,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-consent-custom")
	if err != nil {
		t.Fatalf("get connector by payload: %v", err)
	}
	if !found {
		t.Fatalf("connector not found")
	}

	if err := st.CreateLegalDocument(ctx, domain.LegalDocument{
		Type:      domain.LegalDocumentTypeOffer,
		Title:     "Offer v1",
		Content:   "Offer content",
		Version:   1,
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create offer doc: %v", err)
	}

	h.handleCallback(ctx, &models.CallbackQuery{
		ID:   "cb-consent-custom",
		From: models.User{ID: 1102, Username: "custom_user"},
		Data: "accept_terms:" + int64ToString(connector.ID),
	})

	consent, found, err := st.GetConsent(ctx, 1102, connector.ID)
	if err != nil {
		t.Fatalf("get consent: %v", err)
	}
	if !found {
		t.Fatalf("consent not found")
	}
	if consent.OfferDocumentID != 0 || consent.OfferDocumentVersion != 0 || consent.PrivacyDocumentID != 0 || consent.PrivacyDocumentVersion != 0 {
		t.Fatalf("expected empty consent versions for connector custom URLs, got %+v", consent)
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
	h := NewHandler(st, tg, robokassa, false, "http://localhost:8080", "test-encryption-key-123456789012345")

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

func TestHandlePay_WithExplicitRecurringOptInCreatesRecurringConsent(t *testing.T) {
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
	h := NewHandler(st, tg, robokassa, true, "http://localhost:8080", "test-encryption-key-123456789012345")

	connectorID := seedBotConnector(t, ctx, st, "in-pay-recurring")
	seedRecurringLegalDocs(t, ctx, st)

	h.handlePay(ctx, &models.CallbackQuery{
		ID:   "cb-4",
		From: models.User{ID: 1004},
		Data: "pay:" + int64ToString(connectorID) + ":1",
	})

	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{TelegramID: 1004, Limit: 10})
	if err != nil {
		t.Fatalf("list payments: %v", err)
	}
	if len(payments) != 1 {
		t.Fatalf("payments len = %d, want 1", len(payments))
	}
	if !payments[0].AutoPayEnabled {
		t.Fatalf("payment AutoPayEnabled = false, want true")
	}
	if !strings.Contains(payments[0].CheckoutURL, "Recurring=true") {
		t.Fatalf("checkout URL should contain recurring flag: %s", payments[0].CheckoutURL)
	}

	enabled, hasSettings, err := st.GetUserAutoPayEnabled(ctx, 1004)
	if err != nil {
		t.Fatalf("get user autopay: %v", err)
	}
	if !hasSettings || !enabled {
		t.Fatalf("autopay preference = (%v,%v), want (true,true)", enabled, hasSettings)
	}

	consents, err := st.ListRecurringConsentsByTelegram(ctx, 1004)
	if err != nil {
		t.Fatalf("list recurring consents: %v", err)
	}
	if len(consents) != 1 {
		t.Fatalf("recurring consents len = %d, want 1", len(consents))
	}
	if consents[0].OfferDocumentVersion != 1 || consents[0].UserAgreementDocumentVersion != 1 {
		t.Fatalf("unexpected recurring consent: %+v", consents[0])
	}
}

func TestHandlePay_WithExplicitManualModeOverridesStoredAutopay(t *testing.T) {
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
	h := NewHandler(st, tg, robokassa, true, "http://localhost:8080", "test-encryption-key-123456789012345")

	connectorID := seedBotConnector(t, ctx, st, "in-pay-manual-override")
	seedRecurringLegalDocs(t, ctx, st)
	if err := st.SetUserAutoPayEnabled(ctx, 1005, true, time.Now().UTC()); err != nil {
		t.Fatalf("set user autopay: %v", err)
	}

	h.handlePay(ctx, &models.CallbackQuery{
		ID:   "cb-5",
		From: models.User{ID: 1005},
		Data: "pay:" + int64ToString(connectorID) + ":0",
	})

	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{TelegramID: 1005, Limit: 10})
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

	consents, err := st.ListRecurringConsentsByTelegram(ctx, 1005)
	if err != nil {
		t.Fatalf("list recurring consents: %v", err)
	}
	if len(consents) != 0 {
		t.Fatalf("recurring consents len = %d, want 0", len(consents))
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

func seedRecurringLegalDocs(t *testing.T, ctx context.Context, st *memory.Store) {
	t.Helper()

	if err := st.CreateLegalDocument(ctx, domain.LegalDocument{
		Type:      domain.LegalDocumentTypeOffer,
		Title:     "Offer recurring",
		Content:   "Offer recurring content",
		Version:   1,
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create recurring offer doc: %v", err)
	}
	if err := st.CreateLegalDocument(ctx, domain.LegalDocument{
		Type:      domain.LegalDocumentTypeUserAgreement,
		Title:     "Agreement recurring",
		Content:   "Agreement recurring content",
		Version:   1,
		IsActive:  true,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create recurring agreement doc: %v", err)
	}
}

func TestResolveLegalURL_UsesActiveDocumentFallback(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	h := NewHandler(st, tg, payment.NewMockService("http://localhost:8080"), false, "http://localhost:8080", "test-encryption-key-123456789012345")

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
	want := "http://localhost:8080/oferta/1"
	if got != want {
		t.Fatalf("resolveLegalURL() = %q, want %q", got, want)
	}
}
