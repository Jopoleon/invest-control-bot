package bot

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/payment"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
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
	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "1001", "existing_user")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	user.FullName = "Existing User"
	user.Phone = "+79990001122"
	user.Email = "existing@example.com"
	user.UpdatedAt = time.Now().UTC()
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}

	h.handleCallback(ctx, testAction("cb-1", 1001, "existing_user", "accept_terms:"+int64ToString(connectorID)))

	if state, found, err := st.GetRegistrationState(ctx, domain.MessengerKindTelegram, "1001"); err != nil {
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
	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "1002", "partial_user")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	user.FullName = "Partial User"
	user.Email = "partial@example.com"
	user.UpdatedAt = time.Now().UTC()
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}

	h.handleCallback(ctx, testAction("cb-2", 1002, "partial_user", "accept_terms:"+int64ToString(connectorID)))

	state, found, err := st.GetRegistrationState(ctx, domain.MessengerKindTelegram, "1002")
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

	h.handleCallback(ctx, testAction("cb-consent-versioned", 1101, "versioned_user", "accept_terms:"+int64ToString(connectorID)))

	user, found, err := st.GetUser(ctx, 1101)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !found {
		t.Fatalf("user not found")
	}
	consent, found, err := st.GetConsent(ctx, user.ID, connectorID)
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
		StartPayload:  "in-consent-custom",
		Name:          "custom-consent-connector",
		ChatID:        "1003626584986",
		PriceRUB:      4444,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		OfferURL:      "https://example.com/custom-offer",
		PrivacyURL:    "https://example.com/custom-privacy",
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
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

	h.handleCallback(ctx, testAction("cb-consent-custom", 1102, "custom_user", "accept_terms:"+int64ToString(connector.ID)))

	user, found := domain.User{}, false
	user, found, err = st.GetUser(ctx, 1102)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !found {
		t.Fatalf("user not found")
	}
	consent, found, err := st.GetConsent(ctx, user.ID, connector.ID)
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

func TestHandleCallback_CreatesInternalUserAndTelegramAccount(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	h := NewHandler(st, tg, payment.NewMockService("http://localhost:8080"), false, "http://localhost:8080", "test-encryption-key-123456789012345")

	connectorID := seedBotConnector(t, ctx, st, "in-user-account")

	h.handleCallback(ctx, testAction("cb-user-account", 1201, "linked_user", "accept_terms:"+int64ToString(connectorID)))

	user, found, err := st.GetUser(ctx, 1201)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !found {
		t.Fatalf("user not found")
	}
	if user.ID == 0 {
		t.Fatalf("user id = 0")
	}
	accounts, err := st.ListUserMessengerAccounts(ctx, user.ID)
	if err != nil {
		t.Fatalf("ListUserMessengerAccounts: %v", err)
	}
	if len(accounts) == 0 || accounts[0].Username != "linked_user" {
		t.Fatalf("linked messenger username missing: %+v", accounts)
	}
	if len(accounts) != 1 {
		t.Fatalf("accounts len = %d, want 1", len(accounts))
	}
	if accounts[0].MessengerKind != domain.MessengerKindTelegram || accounts[0].MessengerUserID != "1201" {
		t.Fatalf("unexpected messenger account: %+v", accounts[0])
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

	h.handlePay(ctx, testAction("cb-3", 1003, "", "pay:"+int64ToString(connectorID)))

	user, found, err := st.GetUser(ctx, 1003)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !found {
		t.Fatalf("user not found")
	}
	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{UserID: user.ID, Limit: 10})
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

	h.handlePay(ctx, testAction("cb-4", 1004, "", "pay:"+int64ToString(connectorID)+":1"))

	user, found, err := st.GetUser(ctx, 1004)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !found {
		t.Fatalf("user not found")
	}
	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{UserID: user.ID, Limit: 10})
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

	consents, err := st.ListRecurringConsentsByUser(ctx, user.ID)
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

	h.handlePay(ctx, testAction("cb-5", 1005, "", "pay:"+int64ToString(connectorID)+":0"))

	user, found, err := st.GetUser(ctx, 1005)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if !found {
		t.Fatalf("user not found")
	}
	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{UserID: user.ID, Limit: 10})
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

	consents, err := st.ListRecurringConsentsByUser(ctx, user.ID)
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
		StartPayload:  payload,
		Name:          "test-connector",
		ChatID:        "1003626584986",
		PriceRUB:      4444,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
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

func testAction(id string, userID int64, username, data string) messenger.IncomingAction {
	return messenger.IncomingAction{
		Ref:       actionRef(id),
		User:      userIdentity(userID, username),
		ChatID:    userID,
		MessageID: 1,
		Data:      data,
	}
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
