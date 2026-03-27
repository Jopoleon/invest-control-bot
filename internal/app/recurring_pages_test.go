package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/recurringlink"
	"github.com/Jopoleon/invest-control-bot/internal/store"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestRecurringCheckoutPage_RendersConnectorAndConsent(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-public-recurring")
	if err := st.CreateLegalDocument(ctx, domain.LegalDocument{Type: domain.LegalDocumentTypeOffer, Title: "Offer", Content: "offer", Version: 1, IsActive: true, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("create offer: %v", err)
	}
	if err := st.CreateLegalDocument(ctx, domain.LegalDocument{Type: domain.LegalDocumentTypePrivacy, Title: "Privacy", Content: "privacy", Version: 1, IsActive: true, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("create privacy: %v", err)
	}
	if err := st.CreateLegalDocument(ctx, domain.LegalDocument{Type: domain.LegalDocumentTypeUserAgreement, Title: "Agreement", Content: "agreement", Version: 1, IsActive: true, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("create agreement: %v", err)
	}
	connector, found, err := st.GetConnector(ctx, connectorID)
	if err != nil || !found {
		t.Fatalf("get connector: found=%v err=%v", found, err)
	}
	connector.Description = "Описание тарифа"
	if err := st.SetConnectorActive(ctx, connector.ID, true); err != nil {
		t.Fatalf("ensure connector active: %v", err)
	}

	handler := testRecurringPagesHandler(t, st)
	req := httptest.NewRequest(http.MethodGet, "/subscribe/in-public-recurring", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		body, _ := io.ReadAll(rr.Body)
		t.Fatalf("status=%d body=%q", rr.Code, string(body))
	}
	body, _ := io.ReadAll(rr.Body)
	text := string(body)
	if !strings.Contains(text, "Я согласен на автоматические списания согласно условиям оферты") {
		t.Fatalf("response does not contain recurring consent text: %q", text)
	}
	if !strings.Contains(text, "https://t.me/test_bot?start=in-public-recurring") {
		t.Fatalf("response does not contain bot deeplink: %q", text)
	}
	if !strings.Contains(text, "/start in-public-recurring") {
		t.Fatalf("response does not contain start command for MAX/manual flow: %q", text)
	}
	if !strings.Contains(text, "https://web.max.ru/") {
		t.Fatalf("response does not contain MAX web link: %q", text)
	}
	if strings.Contains(text, "Продолжить оформление в Telegram") {
		t.Fatalf("response still contains telegram-only CTA wording: %q", text)
	}
}

func TestRecurringCancelPage_DisablesAutopay(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-cancel-recurring")
	if err := st.SaveUser(ctx, domain.User{TelegramID: 91001, FullName: "Егор", TelegramUsername: "egor", UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("save user: %v", err)
	}
	if err := st.SetUserAutoPayEnabled(ctx, 91001, true, time.Now().UTC()); err != nil {
		t.Fatalf("enable autopay: %v", err)
	}
	seedPayment(t, ctx, st, domain.Payment{Provider: "robokassa", Status: domain.PaymentStatusPaid, Token: "cancel-test-1", TelegramID: 91001, ConnectorID: connectorID, AmountRUB: 2322, AutoPayEnabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
	paymentRow, found, err := st.GetPaymentByToken(ctx, "cancel-test-1")
	if err != nil || !found {
		t.Fatalf("get payment: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{TelegramID: 91001, ConnectorID: connectorID, PaymentID: paymentRow.ID, Status: domain.SubscriptionStatusActive, AutoPayEnabled: true, StartsAt: time.Now().UTC().Add(-24 * time.Hour), EndsAt: time.Now().UTC().Add(24 * time.Hour), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	token, err := recurringlink.BuildCancelToken("test-encryption-key-12345678901234567890", 91001, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("build cancel token: %v", err)
	}

	handler := testRecurringPagesHandler(t, st)
	getReq := httptest.NewRequest(http.MethodGet, "/unsubscribe/"+token, nil)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		body, _ := io.ReadAll(getRR.Body)
		t.Fatalf("get status=%d body=%q", getRR.Code, string(body))
	}
	getBody, _ := io.ReadAll(getRR.Body)
	if !strings.Contains(string(getBody), "Отключить автоплатеж") {
		t.Fatalf("cancel page does not contain disable action: %q", string(getBody))
	}

	postReq := httptest.NewRequest(http.MethodPost, "/unsubscribe/"+token, strings.NewReader(url.Values{
		"subscription_id": []string{"1"},
	}.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRR := httptest.NewRecorder()
	handler.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusSeeOther {
		body, _ := io.ReadAll(postRR.Body)
		t.Fatalf("post status=%d body=%q", postRR.Code, string(body))
	}
	enabled, _, err := st.GetUserAutoPayEnabled(ctx, 91001)
	if err != nil {
		t.Fatalf("get autopay after disable: %v", err)
	}
	if enabled {
		t.Fatalf("autopay should be disabled after public cancel")
	}
	subs, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{TelegramID: 91001, Status: domain.SubscriptionStatusActive, Limit: 10})
	if err != nil {
		t.Fatalf("list subscriptions after disable: %v", err)
	}
	if len(subs) != 1 || subs[0].AutoPayEnabled {
		t.Fatalf("active subscription autopay should be disabled, got %+v", subs)
	}
}

func TestRecurringCancelPage_SendsConfirmationViaMAX(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	maxSpy := &spySender{}

	connectorID := seedConnector(t, ctx, st, "in-max-cancel-recurring")
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "Федор Николаевич")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}
	if err := st.SetUserAutoPayEnabled(ctx, 193465776, true, time.Now().UTC()); err != nil {
		t.Fatalf("enable autopay: %v", err)
	}
	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "cancel-max-test-1",
		UserID:         maxUser.ID,
		TelegramID:     193465776,
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	})
	paymentRow, found, err := st.GetPaymentByToken(ctx, "cancel-max-test-1")
	if err != nil || !found {
		t.Fatalf("get payment: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         maxUser.ID,
		TelegramID:     193465776,
		ConnectorID:    connectorID,
		PaymentID:      paymentRow.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       time.Now().UTC().Add(-24 * time.Hour),
		EndsAt:         time.Now().UTC().Add(24 * time.Hour),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, 193465776, connectorID)
	if err != nil || !found {
		t.Fatalf("get latest subscription: found=%v err=%v", found, err)
	}
	token, err := recurringlink.BuildCancelToken("test-encryption-key-12345678901234567890", 193465776, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("build cancel token: %v", err)
	}

	handler := testRecurringPagesHandlerWithSenders(t, st, maxSpy)
	postReq := httptest.NewRequest(http.MethodPost, "/unsubscribe/"+token, strings.NewReader(url.Values{
		"subscription_id": []string{strconv.FormatInt(sub.ID, 10)},
	}.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRR := httptest.NewRecorder()
	handler.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusSeeOther {
		body, _ := io.ReadAll(postRR.Body)
		t.Fatalf("post status=%d body=%q", postRR.Code, string(body))
	}
	if len(maxSpy.sent) != 1 {
		t.Fatalf("max sent messages = %d, want 1", len(maxSpy.sent))
	}
	if maxSpy.sent[0].user.Kind != messenger.KindMAX {
		t.Fatalf("sent kind = %s, want %s", maxSpy.sent[0].user.Kind, messenger.KindMAX)
	}
	if got := maxSpy.sent[0].msg.Text; !strings.Contains(got, "Автоплатеж") {
		t.Fatalf("confirmation text = %q, want autopay confirmation", got)
	}
}

func testRecurringPagesHandler(t *testing.T, st store.Store) http.Handler {
	t.Helper()
	cfg := config.Config{
		AppName:     "test-app",
		Environment: config.EnvLocal,
		Runtime:     config.RuntimeServer,
		HTTP:        config.HTTPConfig{Address: ":0", ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second},
		Postgres:    config.PostgresConfig{Driver: "memory"},
		Telegram:    config.TelegramConfig{BotUsername: "test_bot", Webhook: config.WebhookConfig{PublicURL: "https://example.com/telegram/webhook"}},
		Payment:     config.PaymentConfig{Provider: "robokassa", Robokassa: config.RobokassaPaymentConfig{MerchantLogin: "merchant", Password1: "pass1", Password2: "pass2", IsTestMode: true, RecurringEnabled: true}},
		Security:    config.SecurityConfig{AdminToken: "admin-token", EncryptionKey: "test-encryption-key-12345678901234567890"},
	}
	return testServerHandlerWithConfig(t, st, cfg)
}

func testRecurringPagesHandlerWithSenders(t *testing.T, st store.Store, maxSender messenger.Sender) http.Handler {
	t.Helper()
	cfg := config.Config{
		AppName:     "test-app",
		Environment: config.EnvLocal,
		Runtime:     config.RuntimeServer,
		HTTP:        config.HTTPConfig{Address: ":0", ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second},
		Postgres:    config.PostgresConfig{Driver: "memory"},
		Telegram:    config.TelegramConfig{BotUsername: "test_bot", Webhook: config.WebhookConfig{PublicURL: "https://example.com/telegram/webhook"}},
		Payment:     config.PaymentConfig{Provider: "robokassa", Robokassa: config.RobokassaPaymentConfig{MerchantLogin: "merchant", Password1: "pass1", Password2: "pass2", IsTestMode: true, RecurringEnabled: true}},
		Security:    config.SecurityConfig{AdminToken: "admin-token", EncryptionKey: "test-encryption-key-12345678901234567890"},
	}
	appCtx, err := newApplication(cfg, st, appInitOptions{ensureTelegramSetup: false, ensureMAXSetup: false})
	if err != nil {
		t.Fatalf("new application: %v", err)
	}
	if maxSender != nil {
		appCtx.maxSender = maxSender
	}
	return appCtx.newRouter()
}
