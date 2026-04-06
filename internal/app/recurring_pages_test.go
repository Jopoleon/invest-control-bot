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
	if !strings.Contains(text, "Я согласен на автоплатеж согласно условиям оферты") {
		t.Fatalf("response does not contain recurring consent text: %q", text)
	}
	if !strings.Contains(text, "https://t.me/test_bot?start=in-public-recurring") {
		t.Fatalf("response does not contain bot deeplink: %q", text)
	}
	if !strings.Contains(text, "/start in-public-recurring") {
		t.Fatalf("response does not contain start command for MAX/manual flow: %q", text)
	}
	if !strings.Contains(text, "https://max.ru/id9718272494_bot?start=in-public-recurring") {
		t.Fatalf("response does not contain direct MAX bot deeplink: %q", text)
	}
	if !strings.Contains(text, "https://max.ru/id9718272494_bot") {
		t.Fatalf("response does not contain MAX bot chat link: %q", text)
	}
	if !strings.Contains(text, "Открыть MAX") {
		t.Fatalf("response does not contain MAX web fallback action: %q", text)
	}
	if strings.Contains(text, "Продолжить оформление в Telegram") {
		t.Fatalf("response still contains telegram-only CTA wording: %q", text)
	}
}

func TestRecurringCancelPage_DisablesAutopay(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-cancel-recurring")
	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "91001", "egor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	user.FullName = "Егор"
	user.UpdatedAt = time.Now().UTC()
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("save user: %v", err)
	}
	seedPayment(t, ctx, st, domain.Payment{Provider: "robokassa", Status: domain.PaymentStatusPaid, Token: "cancel-test-1", UserID: seedTelegramUser(t, ctx, st, 91001), ConnectorID: connectorID, AmountRUB: 2322, AutoPayEnabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()})
	paymentRow, found, err := st.GetPaymentByToken(ctx, "cancel-test-1")
	if err != nil || !found {
		t.Fatalf("get payment: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{UserID: seedTelegramUser(t, ctx, st, 91001), ConnectorID: connectorID, PaymentID: paymentRow.ID, Status: domain.SubscriptionStatusActive, AutoPayEnabled: true, StartsAt: time.Now().UTC().Add(-24 * time.Hour), EndsAt: time.Now().UTC().Add(24 * time.Hour), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}); err != nil {
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
	if !strings.Contains(string(getBody), "Остановить автосписания по этой подписке") {
		t.Fatalf("cancel page does not contain per-subscription disable action: %q", string(getBody))
	}
	if !strings.Contains(string(getBody), "Уже оплаченный доступ") {
		t.Fatalf("cancel page does not explain paid access retention: %q", string(getBody))
	}
	if !strings.Contains(string(getBody), "Пользователь") {
		t.Fatalf("cancel page does not contain enriched user meta: %q", string(getBody))
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
	subs, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{UserID: seedTelegramUser(t, ctx, st, 91001), Status: domain.SubscriptionStatusActive, Limit: 10})
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
	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "cancel-max-test-1",
		UserID:         maxUser.ID,
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
	subscriptions, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{UserID: maxUser.ID, Limit: 10})
	if err != nil || len(subscriptions) == 0 {
		t.Fatalf("list subscriptions: len=%d err=%v", len(subscriptions), err)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, subscriptions[0].UserID, connectorID)
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

func TestRecurringCancelPage_ShowsCurrentAndFutureSubscriptionsSeparately(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-cancel-current-future")
	userID := seedTelegramUser(t, ctx, st, 91003)
	now := time.Now().UTC()

	seedPayment(t, ctx, st, domain.Payment{Provider: "robokassa", Status: domain.PaymentStatusPaid, Token: "cancel-split-1", UserID: userID, ConnectorID: connectorID, AmountRUB: 2322, AutoPayEnabled: true, CreatedAt: now, UpdatedAt: now})
	parentPayment, found, err := st.GetPaymentByToken(ctx, "cancel-split-1")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         userID,
		ConnectorID:    connectorID,
		PaymentID:      parentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(-time.Hour),
		EndsAt:         now.Add(time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment current: %v", err)
	}

	seedPayment(t, ctx, st, domain.Payment{Provider: "robokassa", Status: domain.PaymentStatusPaid, Token: "cancel-split-2", UserID: userID, ConnectorID: connectorID, AmountRUB: 2322, AutoPayEnabled: true, CreatedAt: now, UpdatedAt: now})
	futurePayment, found, err := st.GetPaymentByToken(ctx, "cancel-split-2")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken future found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         userID,
		ConnectorID:    connectorID,
		PaymentID:      futurePayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(time.Hour),
		EndsAt:         now.Add(2 * time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment future: %v", err)
	}

	token, err := recurringlink.BuildCancelToken("test-encryption-key-12345678901234567890", 91003, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("build cancel token: %v", err)
	}

	handler := testRecurringPagesHandler(t, st)
	req := httptest.NewRequest(http.MethodGet, "/unsubscribe/"+token, nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Следующий период") {
		t.Fatalf("body does not contain future section: %q", body)
	}
	if !strings.Contains(body, "Ожидает начала") {
		t.Fatalf("body does not contain future start label: %q", body)
	}
	if !strings.Contains(body, "<strong>2</strong>") {
		t.Fatalf("body does not contain autopay count for current+future periods: %q", body)
	}
	if !strings.Contains(body, "<small>Активных доступов</small>") || !strings.Contains(body, "<strong>1</strong>") {
		t.Fatalf("body does not contain active access count for current period only: %q", body)
	}
}

func TestRecurringCancelPage_ShowsReturnToMAXBotLink(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-max-cancel-return")
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465780", "fedor")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}
	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "cancel-max-return-1",
		UserID:         maxUser.ID,
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	})
	paymentRow, found, err := st.GetPaymentByToken(ctx, "cancel-max-return-1")
	if err != nil || !found {
		t.Fatalf("get payment: found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         maxUser.ID,
		ConnectorID:    connectorID,
		PaymentID:      paymentRow.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       time.Now().UTC().Add(-time.Hour),
		EndsAt:         time.Now().UTC().Add(time.Hour),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	token, err := recurringlink.BuildCancelToken("test-encryption-key-12345678901234567890", 193465780, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("build cancel token: %v", err)
	}

	handler := testRecurringPagesHandler(t, st)
	req := httptest.NewRequest(http.MethodGet, "/unsubscribe/"+token, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		body, _ := io.ReadAll(rec.Body)
		t.Fatalf("status=%d body=%q", rec.Code, string(body))
	}
	body, _ := io.ReadAll(rec.Body)
	text := string(body)
	if !strings.Contains(text, "Открыть бота в MAX") {
		t.Fatalf("cancel page does not contain MAX return action: %q", text)
	}
	if !strings.Contains(text, "https://max.ru/id9718272494_bot") {
		t.Fatalf("cancel page does not contain MAX bot chat url: %q", text)
	}
}

func TestRecurringCancelPage_ShowsDetailedSuccessState(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-cancel-success-state")
	userID := seedTelegramUser(t, ctx, st, 91002)
	user, found, err := st.GetUserByID(ctx, userID)
	if err != nil || !found {
		t.Fatalf("GetUserByID found=%v err=%v", found, err)
	}
	user.FullName = "Егор Тест"
	user.UpdatedAt = time.Now().UTC()
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser err=%v", err)
	}
	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "cancel-success-state-1",
		UserID:         userID,
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: false,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	})
	paymentRow, found, err := st.GetPaymentByToken(ctx, "cancel-success-state-1")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         userID,
		ConnectorID:    connectorID,
		PaymentID:      paymentRow.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: false,
		StartsAt:       time.Now().UTC().Add(-24 * time.Hour),
		EndsAt:         time.Now().UTC().Add(24 * time.Hour),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment err=%v", err)
	}
	token, err := recurringlink.BuildCancelToken("test-encryption-key-12345678901234567890", 91002, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("build cancel token: %v", err)
	}

	handler := testRecurringPagesHandler(t, st)
	req := httptest.NewRequest(http.MethodGet, "/unsubscribe/"+token+"?done=%D1%82%D0%B5%D1%81%D1%82-%D0%BA%D0%BE%D0%BD%D0%BD%D0%B5%D0%BA%D1%82%D0%BE%D1%80&state=stale_success", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		body, _ := io.ReadAll(rec.Body)
		t.Fatalf("status=%d body=%q", rec.Code, string(body))
	}
	body, _ := io.ReadAll(rec.Body)
	text := string(body)
	if !strings.Contains(text, "Автосписания остановлены") {
		t.Fatalf("success page does not contain strong success heading: %q", text)
	}
	if !strings.Contains(text, "Подписка обновилась") {
		t.Fatalf("success page does not contain stale-success state label: %q", text)
	}
	if !strings.Contains(text, "Автоплатеж для подписки") {
		t.Fatalf("success page does not contain named subscription confirmation: %q", text)
	}
}

func TestRecurringCancelPage_ShowsExpiredLinkState(t *testing.T) {
	handler := testRecurringPagesHandler(t, memory.New())
	token, err := recurringlink.BuildCancelToken("test-encryption-key-12345678901234567890", 91003, time.Now().UTC().Add(-time.Minute))
	if err != nil {
		t.Fatalf("build cancel token: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/unsubscribe/"+token, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusGone {
		body, _ := io.ReadAll(rec.Body)
		t.Fatalf("status=%d body=%q", rec.Code, string(body))
	}
	body, _ := io.ReadAll(rec.Body)
	text := string(body)
	if !strings.Contains(text, "Ссылка истекла") {
		t.Fatalf("expired-link page does not contain dedicated state label: %q", text)
	}
	if !strings.Contains(text, "Нужна новая ссылка") {
		t.Fatalf("expired-link page does not contain renewal guidance: %q", text)
	}
}

func TestRecurringCancelPage_ShowsAlreadyOffState(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-cancel-already-off")
	userID := seedTelegramUser(t, ctx, st, 91004)
	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "cancel-already-off-1",
		UserID:         userID,
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: false,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	})
	paymentRow, found, err := st.GetPaymentByToken(ctx, "cancel-already-off-1")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         userID,
		ConnectorID:    connectorID,
		PaymentID:      paymentRow.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: false,
		StartsAt:       time.Now().UTC().Add(-time.Hour),
		EndsAt:         time.Now().UTC().Add(time.Hour),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment err=%v", err)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, userID, connectorID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}
	token, err := recurringlink.BuildCancelToken("test-encryption-key-12345678901234567890", 91004, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("build cancel token: %v", err)
	}

	handler := testRecurringPagesHandler(t, st)
	postReq := httptest.NewRequest(http.MethodPost, "/unsubscribe/"+token, strings.NewReader(url.Values{
		"subscription_id": []string{strconv.FormatInt(sub.ID, 10)},
	}.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRR := httptest.NewRecorder()
	handler.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		body, _ := io.ReadAll(postRR.Body)
		t.Fatalf("status=%d body=%q", postRR.Code, string(body))
	}
	body, _ := io.ReadAll(postRR.Body)
	text := string(body)
	if !strings.Contains(text, "Уже отключено") {
		t.Fatalf("already-off page does not contain dedicated state label: %q", text)
	}
	if !strings.Contains(text, "автоплатеж уже выключен") {
		t.Fatalf("already-off page does not contain explicit already-off message: %q", text)
	}
}

func TestRecurringCancelPage_ShowsStaleSubmitState(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-cancel-stale-submit")
	userID := seedTelegramUser(t, ctx, st, 91005)
	now := time.Now().UTC()

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "cancel-stale-submit-old",
		UserID:         userID,
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      now.Add(-2 * time.Hour),
		UpdatedAt:      now.Add(-2 * time.Hour),
	})
	oldPayment, found, err := st.GetPaymentByToken(ctx, "cancel-stale-submit-old")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken old found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         userID,
		ConnectorID:    connectorID,
		PaymentID:      oldPayment.ID,
		Status:         domain.SubscriptionStatusExpired,
		AutoPayEnabled: true,
		StartsAt:       now.Add(-3 * time.Hour),
		EndsAt:         now.Add(-90 * time.Minute),
		CreatedAt:      now.Add(-3 * time.Hour),
		UpdatedAt:      now.Add(-90 * time.Minute),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment old err=%v", err)
	}
	oldSub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, userID, connectorID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector old found=%v err=%v", found, err)
	}

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "cancel-stale-submit-new",
		UserID:         userID,
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: false,
		CreatedAt:      now.Add(-30 * time.Minute),
		UpdatedAt:      now.Add(-30 * time.Minute),
	})
	newPayment, found, err := st.GetPaymentByToken(ctx, "cancel-stale-submit-new")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken new found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         userID,
		ConnectorID:    connectorID,
		PaymentID:      newPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: false,
		StartsAt:       now.Add(-30 * time.Minute),
		EndsAt:         now.Add(90 * time.Minute),
		CreatedAt:      now.Add(-30 * time.Minute),
		UpdatedAt:      now.Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment new err=%v", err)
	}

	token, err := recurringlink.BuildCancelToken("test-encryption-key-12345678901234567890", 91005, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("build cancel token: %v", err)
	}

	handler := testRecurringPagesHandler(t, st)
	postReq := httptest.NewRequest(http.MethodPost, "/unsubscribe/"+token, strings.NewReader(url.Values{
		"subscription_id": []string{strconv.FormatInt(oldSub.ID, 10)},
	}.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRR := httptest.NewRecorder()
	handler.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		body, _ := io.ReadAll(postRR.Body)
		t.Fatalf("status=%d body=%q", postRR.Code, string(body))
	}
	body, _ := io.ReadAll(postRR.Body)
	text := string(body)
	if !strings.Contains(text, "Страница устарела") {
		t.Fatalf("stale-submit page does not contain dedicated state label: %q", text)
	}
	if !strings.Contains(text, "Обновите страницу из бота") {
		t.Fatalf("stale-submit page does not contain refresh guidance: %q", text)
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
		MAX:         config.MAXConfig{BotUsername: "id9718272494_bot"},
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
		MAX:         config.MAXConfig{BotUsername: "id9718272494_bot"},
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
