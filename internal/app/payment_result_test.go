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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
)

type spySender struct {
	sent []spySentMessage
}

type spySentMessage struct {
	user messenger.UserRef
	msg  messenger.OutgoingMessage
}

func (s *spySender) Send(_ context.Context, user messenger.UserRef, msg messenger.OutgoingMessage) error {
	s.sent = append(s.sent, spySentMessage{user: user, msg: msg})
	return nil
}

func (s *spySender) Edit(context.Context, messenger.MessageRef, messenger.OutgoingMessage) error {
	return nil
}

func (s *spySender) AnswerAction(context.Context, messenger.ActionRef, string) error {
	return nil
}

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
		UserID:      seedTelegramUser(t, ctx, st, 777001),
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
		UserID: paymentRow.UserID,
		Status: domain.SubscriptionStatusActive,
		Limit:  20,
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

	if got := countAuditEvents(events, domain.AuditActionPaymentSuccessNotified); got != 1 {
		t.Fatalf("payment_success_notified count = %d, want 1", got)
	}
	if got := countAuditEvents(events, domain.AuditActionRobokassaResultReceived); got != 2 {
		t.Fatalf("robokassa_result_received count = %d, want 2", got)
	}
}

func TestActivateSuccessfulPayment_SendsSuccessMessageViaMAXAccount(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	maxSpy := &spySender{}

	connectorID := seedConnector(t, ctx, st, "in-max-success")
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "Федор Николаевич")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}

	now := time.Now().UTC()
	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPending,
		Token:          "max-success-1",
		UserID:         maxUser.ID,
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      now,
		UpdatedAt:      now,
	})

	paymentRow, found, err := st.GetPaymentByToken(ctx, "max-success-1")
	if err != nil || !found {
		t.Fatalf("get payment by token: found=%v err=%v", found, err)
	}

	appCtx := &application{
		store:          st,
		telegramClient: tg,
		maxSender:      maxSpy,
	}
	appCtx.activateSuccessfulPayment(ctx, paymentRow, "robokassa:max-success-1", now)

	if len(maxSpy.sent) != 1 {
		t.Fatalf("max sent messages = %d, want 1", len(maxSpy.sent))
	}
	if maxSpy.sent[0].user.Kind != messenger.KindMAX {
		t.Fatalf("sent kind = %s, want %s", maxSpy.sent[0].user.Kind, messenger.KindMAX)
	}
	if maxSpy.sent[0].user.UserID != 193465776 {
		t.Fatalf("sent user id = %d, want 193465776", maxSpy.sent[0].user.UserID)
	}
	if got := maxSpy.sent[0].msg.Text; !strings.Contains(got, "Оплата прошла успешно") {
		t.Fatalf("success text = %q, want payment success notification", got)
	}
	if got := maxSpy.sent[0].msg.Text; !strings.Contains(got, "test-connector") {
		t.Fatalf("success text = %q, want connector name", got)
	}
	if got := maxSpy.sent[0].msg.Text; !strings.Contains(got, "2322 ₽") {
		t.Fatalf("success text = %q, want amount", got)
	}
	if got := maxSpy.sent[0].msg.Text; !strings.Contains(got, "Автоплатеж") {
		t.Fatalf("success text = %q, want autopay hint", got)
	}
	if got := len(maxSpy.sent[0].msg.Buttons); got != 2 {
		t.Fatalf("button rows = %d, want 2", got)
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionPaymentSuccessNotified); got != 1 {
		t.Fatalf("payment_success_notified count = %d, want 1", got)
	}
}

func TestActivateSuccessfulPayment_ExtendsFromCurrentSubscriptionEnd(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}

	connectorID := seedConnector(t, ctx, st, "in-extend-current-period")
	now := time.Now().UTC()
	currentEnd := now.Add(5 * 24 * time.Hour).Truncate(time.Second)
	seedPayment(t, ctx, st, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "extend-current-period-1",
		UserID:      seedTelegramUser(t, ctx, st, 777777),
		ConnectorID: connectorID,
		AmountRUB:   2322,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	paymentRow, found, err := st.GetPaymentByToken(ctx, "extend-current-period-1")
	if err != nil || !found {
		t.Fatalf("get payment by token: found=%v err=%v", found, err)
	}

	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:      paymentRow.UserID,
		ConnectorID: paymentRow.ConnectorID,
		PaymentID:   999999,
		Status:      domain.SubscriptionStatusActive,
		StartsAt:    now.Add(-24 * time.Hour),
		EndsAt:      currentEnd,
		CreatedAt:   now.Add(-24 * time.Hour),
		UpdatedAt:   now.Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("seed current subscription: %v", err)
	}

	appCtx := &application{
		store:          st,
		telegramClient: tg,
	}
	appCtx.activateSuccessfulPayment(ctx, paymentRow, "robokassa:extend-current-period-1", now)

	latestSub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, paymentRow.UserID, paymentRow.ConnectorID)
	if err != nil || !found {
		t.Fatalf("get latest subscription: found=%v err=%v", found, err)
	}
	if latestSub.PaymentID != paymentRow.ID {
		t.Fatalf("latest subscription payment_id=%d want=%d", latestSub.PaymentID, paymentRow.ID)
	}
	if !latestSub.StartsAt.Equal(currentEnd) {
		t.Fatalf("starts_at=%s want=%s", latestSub.StartsAt, currentEnd)
	}
	wantEnd := currentEnd.AddDate(0, 0, 30)
	if !latestSub.EndsAt.Equal(wantEnd) {
		t.Fatalf("ends_at=%s want=%s", latestSub.EndsAt, wantEnd)
	}
}

func TestActivateSuccessfulPayment_UsesShortTestPeriodWhenConfigured(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	userID := seedTelegramUser(t, ctx, st, 888888)
	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-short-period",
		Name:          "short-period-test",
		ChatID:        "1003626584986",
		PriceRUB:      2322,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 90,
		IsActive:      true,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-short-period")
	if err != nil || !found {
		t.Fatalf("get connector by payload: found=%v err=%v", found, err)
	}

	seedPayment(t, ctx, st, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "short-period-payment-1",
		UserID:      userID,
		ConnectorID: connector.ID,
		AmountRUB:   2322,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	paymentRow, found, err := st.GetPaymentByToken(ctx, "short-period-payment-1")
	if err != nil || !found {
		t.Fatalf("get payment by token: found=%v err=%v", found, err)
	}

	appCtx := &application{
		store:          st,
		telegramClient: tg,
	}
	appCtx.activateSuccessfulPayment(ctx, paymentRow, "robokassa:short-period-payment-1", now)

	latestSub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, paymentRow.UserID, paymentRow.ConnectorID)
	if err != nil || !found {
		t.Fatalf("get latest subscription: found=%v err=%v", found, err)
	}
	wantEnd := now.Add(90 * time.Second)
	if !latestSub.StartsAt.Equal(now) {
		t.Fatalf("starts_at=%s want=%s", latestSub.StartsAt, now)
	}
	if !latestSub.EndsAt.Equal(wantEnd) {
		t.Fatalf("ends_at=%s want=%s", latestSub.EndsAt, wantEnd)
	}
}

func TestActivateSuccessfulPayment_DoesNotSendDuplicateSuccessNotificationForAlreadyPaidPayment(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	maxSpy := &spySender{}

	connectorID := seedConnector(t, ctx, st, "in-paid-no-duplicate-notify")
	now := time.Now().UTC()
	paidAt := now.Add(-time.Hour)
	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "already-paid-1",
		UserID:         seedTelegramUser(t, ctx, st, 193465776),
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		PaidAt:         &paidAt,
		CreatedAt:      paidAt,
		UpdatedAt:      paidAt,
	})
	paymentRow, found, err := st.GetPaymentByToken(ctx, "already-paid-1")
	if err != nil || !found {
		t.Fatalf("get payment by token: found=%v err=%v", found, err)
	}

	appCtx := &application{
		store:          st,
		telegramClient: tg,
		maxSender:      maxSpy,
	}
	appCtx.activateSuccessfulPayment(ctx, paymentRow, "robokassa:already-paid-1", now)

	if len(maxSpy.sent) != 0 {
		t.Fatalf("max notifications=%d want=0", len(maxSpy.sent))
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionPaymentSuccessNotified); got != 0 {
		t.Fatalf("payment_success_notified count=%d want=0", got)
	}
}

func TestBuildPaymentPageActions_SelectsMessengerSpecificActions(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()

	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "Федор Николаевич")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}

	appCtx := &application{store: st}

	maxActions := appCtx.buildPaymentPageActions(ctx, domain.Payment{
		UserID: maxUser.ID,
	}, "https://web.max.ru/-72598909498032", true)
	if len(maxActions) != 2 {
		t.Fatalf("max actions len=%d want=2", len(maxActions))
	}
	if maxActions[0].Label != appPaymentActionOpenMAXChannel {
		t.Fatalf("max first label=%q want=%q", maxActions[0].Label, appPaymentActionOpenMAXChannel)
	}
	if maxActions[1].Label != appPaymentActionOpenMAX {
		t.Fatalf("max second label=%q want=%q", maxActions[1].Label, appPaymentActionOpenMAX)
	}

	tgActions := appCtx.buildPaymentPageActions(ctx, domain.Payment{
		UserID: seedTelegramUser(t, ctx, st, 777123),
	}, "https://t.me/example_channel", false)
	if len(tgActions) != 3 {
		t.Fatalf("telegram actions len=%d want=3", len(tgActions))
	}
	if tgActions[0].Label != appPaymentActionReturnToBot {
		t.Fatalf("telegram first label=%q want=%q", tgActions[0].Label, appPaymentActionReturnToBot)
	}
	if tgActions[1].Label != appPaymentActionOpenChannel {
		t.Fatalf("telegram second label=%q want=%q", tgActions[1].Label, appPaymentActionOpenChannel)
	}
	if tgActions[2].Label != appPaymentActionOpenTelegram {
		t.Fatalf("telegram third label=%q want=%q", tgActions[2].Label, appPaymentActionOpenTelegram)
	}
}

func TestNotifyFailedRecurringPayment_SendsMAXNotification(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("create telegram client: %v", err)
	}
	maxSpy := &spySender{}

	connectorID := seedConnector(t, ctx, st, "in-recurring-failed-max")
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "Федор Николаевич")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}

	appCtx := &application{
		config: config.Config{
			Telegram: config.TelegramConfig{BotUsername: "friendly_111_neighbour_bot"},
		},
		store:          st,
		telegramClient: tg,
		maxSender:      maxSpy,
	}

	appCtx.notifyFailedRecurringPayment(ctx, domain.Payment{
		ID:             90,
		UserID:         maxUser.ID,
		ConnectorID:    connectorID,
		SubscriptionID: 12,
		AutoPayEnabled: true,
	})

	if len(maxSpy.sent) != 1 {
		t.Fatalf("max sent=%d want=1", len(maxSpy.sent))
	}
	if maxSpy.sent[0].user.Kind != messenger.KindMAX {
		t.Fatalf("kind=%s want=%s", maxSpy.sent[0].user.Kind, messenger.KindMAX)
	}
	if got := maxSpy.sent[0].msg.Text; !strings.Contains(got, "Автоматическое списание не прошло") {
		t.Fatalf("text=%q want recurring failure notification", got)
	}
	if got := len(maxSpy.sent[0].msg.Buttons); got != 1 {
		t.Fatalf("button rows=%d want=1", got)
	}
}

func TestPaymentSuccessPage_MAXActionsUseMAXLinks(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "Федор Николаевич")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}

	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-max-page-success",
		Name:          "max-page-connector",
		ChannelURL:    "https://web.max.ru/-72598909498032",
		PriceRUB:      2322,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-max-page-success")
	if err != nil || !found {
		t.Fatalf("get connector by payload: found=%v err=%v", found, err)
	}

	seedPayment(t, ctx, st, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPaid,
		Token:       "max-page-success-1",
		UserID:      maxUser.ID,
		ConnectorID: connector.ID,
		AmountRUB:   2322,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	})

	handler := testServerHandler(t, st, "test-pass2")
	req := httptest.NewRequest(http.MethodGet, "/payment/success?InvId=max-page-success-1", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		body, _ := io.ReadAll(rr.Body)
		t.Fatalf("status=%d body=%q", rr.Code, string(body))
	}
	body, _ := io.ReadAll(rr.Body)
	text := string(body)
	if !strings.Contains(text, "https://web.max.ru/-72598909498032") {
		t.Fatalf("response does not contain MAX channel URL: %q", text)
	}
	if !strings.Contains(text, "https://web.max.ru/") {
		t.Fatalf("response does not contain MAX web URL: %q", text)
	}
	if strings.Contains(text, "https://t.me") {
		t.Fatalf("response should not contain Telegram URLs for MAX payment page: %q", text)
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
		UserID:      seedTelegramUser(t, ctx, st, 777002),
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
		UserID: paymentRow.UserID,
		Limit:  20,
	})
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("subscriptions = %d, want 0", len(subs))
	}
}

func TestPaymentResult_RejectsInvalidResultSignature(t *testing.T) {
	t.Helper()

	const (
		pass2 = "test-pass2"
		invID = "1000000003"
	)

	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-result-bad-signature")
	seedPayment(t, ctx, st, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       invID,
		UserID:      seedTelegramUser(t, ctx, st, 777003),
		ConnectorID: connectorID,
		AmountRUB:   2322,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	})

	handler := testServerHandler(t, st, pass2)
	code, _ := postPaymentResult(t, handler, "2322.00", invID, "bad-signature")
	if code != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want %d", code, http.StatusBadRequest)
	}

	paymentRow, found, err := st.GetPaymentByToken(ctx, invID)
	if err != nil || !found {
		t.Fatalf("get payment by token: found=%v err=%v", found, err)
	}
	if paymentRow.Status != domain.PaymentStatusPending {
		t.Fatalf("payment status = %s, want %s", paymentRow.Status, domain.PaymentStatusPending)
	}
}

func TestPaymentSuccessPage_RejectsInvalidSuccessSignature(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-success-bad-signature")
	seedPayment(t, ctx, st, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "success-bad-signature-1",
		UserID:      seedTelegramUser(t, ctx, st, 777004),
		ConnectorID: connectorID,
		AmountRUB:   2322,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	})

	handler := testServerHandler(t, st, "test-pass2")
	req := httptest.NewRequest(http.MethodGet, "/payment/success?OutSum=2322.00&InvId=success-bad-signature-1&SignatureValue=bad-signature", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		body, _ := io.ReadAll(rr.Body)
		t.Fatalf("status=%d body=%q", rr.Code, string(body))
	}
}

func TestPaymentFailPage_MAXActionsUseMAXLinks(t *testing.T) {
	t.Helper()

	ctx := context.Background()
	st := memory.New()
	maxUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "Федор Николаевич")
	if err != nil {
		t.Fatalf("create max user: %v", err)
	}

	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-max-page-fail",
		Name:          "max-fail-connector",
		ChannelURL:    "https://web.max.ru/-72598909498032",
		PriceRUB:      2322,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create connector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-max-page-fail")
	if err != nil || !found {
		t.Fatalf("get connector by payload: found=%v err=%v", found, err)
	}

	seedPayment(t, ctx, st, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPending,
		Token:       "max-page-fail-1",
		UserID:      maxUser.ID,
		ConnectorID: connector.ID,
		AmountRUB:   2322,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	})

	handler := testServerHandler(t, st, "test-pass2")
	req := httptest.NewRequest(http.MethodGet, "/payment/fail?InvId=max-page-fail-1", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		body, _ := io.ReadAll(rr.Body)
		t.Fatalf("status=%d body=%q", rr.Code, string(body))
	}
	body, _ := io.ReadAll(rr.Body)
	text := string(body)
	if !strings.Contains(text, "https://web.max.ru/-72598909498032") {
		t.Fatalf("response does not contain MAX channel URL: %q", text)
	}
	if !strings.Contains(text, "https://web.max.ru/") {
		t.Fatalf("response does not contain MAX web URL: %q", text)
	}
	if strings.Contains(text, "https://t.me") {
		t.Fatalf("response should not contain Telegram URLs for MAX fail page: %q", text)
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
		UserID:         seedTelegramUser(t, ctx, st, 778001),
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
		UserID:         parentPayment.UserID,
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
		UserID: parentPayment.UserID,
		Limit:  20,
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

func TestPaymentRebill_UsesRootRecurringInvoiceAsPreviousInvoiceID(t *testing.T) {
	t.Helper()

	const (
		adminToken = "admin-secret"
		pass2      = "test-pass2"
		rootInv    = "1000010001"
		childInv   = "1000010002"
	)

	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-rebill-root-parent")

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          rootInv,
		UserID:         seedTelegramUser(t, ctx, st, 778002),
		ConnectorID:    connectorID,
		AmountRUB:      2322,
		AutoPayEnabled: true,
		CreatedAt:      time.Now().UTC().Add(-48 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-48 * time.Hour),
	})
	rootPayment, found, err := st.GetPaymentByToken(ctx, rootInv)
	if err != nil || !found {
		t.Fatalf("root payment not found: found=%v err=%v", found, err)
	}

	seedPayment(t, ctx, st, domain.Payment{
		Provider:          "robokassa",
		ProviderPaymentID: "robokassa:" + childInv,
		Status:            domain.PaymentStatusPaid,
		Token:             childInv,
		UserID:            rootPayment.UserID,
		ConnectorID:       rootPayment.ConnectorID,
		ParentPaymentID:   rootPayment.ID,
		AmountRUB:         2322,
		AutoPayEnabled:    true,
		CreatedAt:         time.Now().UTC().Add(-24 * time.Hour),
		PaidAt:            func() *time.Time { v := time.Now().UTC().Add(-24 * time.Hour); return &v }(),
		UpdatedAt:         time.Now().UTC().Add(-24 * time.Hour),
	})
	childPayment, found, err := st.GetPaymentByToken(ctx, childInv)
	if err != nil || !found {
		t.Fatalf("child payment not found: found=%v err=%v", found, err)
	}

	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         childPayment.UserID,
		ConnectorID:    childPayment.ConnectorID,
		PaymentID:      childPayment.ID,
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
		UserID: childPayment.UserID,
		Limit:  20,
	})
	if err != nil || len(subscriptions) != 1 {
		t.Fatalf("expected one subscription, got=%d err=%v", len(subscriptions), err)
	}

	rebillMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if got := r.FormValue("PreviousInvoiceID"); got != rootInv {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("unexpected parent: " + got))
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
		Postgres: config.PostgresConfig{Driver: "memory"},
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
		Security: config.SecurityConfig{AdminToken: adminToken},
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
}

func TestPaymentRebill_ReturnsExistingPendingPayment(t *testing.T) {
	t.Helper()

	const (
		adminToken = "admin-secret"
		pass2      = "test-pass2"
		parentInv  = "1000009002"
	)

	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-rebill-existing")

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          parentInv,
		UserID:         seedTelegramUser(t, ctx, st, 778002),
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
		UserID:         parentPayment.UserID,
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
	subscriptions, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{UserID: parentPayment.UserID, Limit: 20})
	if err != nil || len(subscriptions) != 1 {
		t.Fatalf("expected one subscription, got=%d err=%v", len(subscriptions), err)
	}

	existingInvoice := "1000009003"
	seedPayment(t, ctx, st, domain.Payment{
		Provider:          "robokassa",
		Status:            domain.PaymentStatusPending,
		Token:             existingInvoice,
		UserID:            parentPayment.UserID,
		ConnectorID:       parentPayment.ConnectorID,
		SubscriptionID:    subscriptions[0].ID,
		ParentPaymentID:   parentPayment.ID,
		AmountRUB:         2322,
		AutoPayEnabled:    true,
		ProviderPaymentID: "rebill_parent:" + parentInv,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	})

	rebillCalls := 0
	rebillMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rebillCalls++
		_, _ = w.Write([]byte("OK+should-not-be-called"))
	}))
	defer rebillMock.Close()

	cfg := config.Config{
		AppName:     "telega-bot-fedor-test",
		Environment: config.EnvLocal,
		HTTP:        config.HTTPConfig{Address: ":0", ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second},
		Postgres:    config.PostgresConfig{Driver: "memory"},
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
		Security: config.SecurityConfig{AdminToken: adminToken},
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
		Existing  bool   `json:"existing"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("decode rebill response: %v", err)
	}
	if !payload.OK || !payload.Existing || payload.InvoiceID != existingInvoice {
		t.Fatalf("unexpected rebill payload: %+v", payload)
	}
	if rebillCalls != 0 {
		t.Fatalf("rebill provider called %d times, want 0", rebillCalls)
	}
}

func TestPaymentRebill_MarksPaymentFailedWhenProviderFails(t *testing.T) {
	t.Helper()

	const (
		adminToken = "admin-secret"
		pass2      = "test-pass2"
		parentInv  = "1000009010"
	)

	ctx := context.Background()
	st := memory.New()
	connectorID := seedConnector(t, ctx, st, "in-rebill-fail")

	seedPayment(t, ctx, st, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          parentInv,
		UserID:         seedTelegramUser(t, ctx, st, 778010),
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
		UserID:         parentPayment.UserID,
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
	subscriptions, err := st.ListSubscriptions(ctx, domain.SubscriptionListQuery{UserID: parentPayment.UserID, Limit: 20})
	if err != nil || len(subscriptions) != 1 {
		t.Fatalf("expected one subscription, got=%d err=%v", len(subscriptions), err)
	}

	rebillMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer rebillMock.Close()

	cfg := config.Config{
		AppName:     "telega-bot-fedor-test",
		Environment: config.EnvLocal,
		HTTP:        config.HTTPConfig{Address: ":0", ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second},
		Postgres:    config.PostgresConfig{Driver: "memory"},
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
		Security: config.SecurityConfig{AdminToken: adminToken},
	}
	handler := testServerHandlerWithConfig(t, st, cfg)

	form := url.Values{"subscription_id": {fmt.Sprintf("%d", subscriptions[0].ID)}}
	req := httptest.NewRequest(http.MethodPost, "/payment/rebill", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		body, _ := io.ReadAll(rr.Body)
		t.Fatalf("rebill status=%d body=%q", rr.Code, string(body))
	}

	payments, err := st.ListPayments(ctx, domain.PaymentListQuery{UserID: parentPayment.UserID, Limit: 20})
	if err != nil {
		t.Fatalf("list payments: %v", err)
	}
	failedRebillCount := 0
	for _, item := range payments {
		if item.SubscriptionID == subscriptions[0].ID && item.Status == domain.PaymentStatusFailed {
			failedRebillCount++
		}
	}
	if failedRebillCount != 1 {
		t.Fatalf("failed rebill payments=%d want=1", failedRebillCount)
	}

	events, _, err := st.ListAuditEvents(ctx, domain.AuditEventListQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if got := countAuditEvents(events, domain.AuditActionRebillRequestFailed); got != 1 {
		t.Fatalf("rebill_request_failed count=%d want=1", got)
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
		StartPayload:  payload,
		Name:          "test-connector",
		ChatID:        "1003626584986",
		PriceRUB:      2322,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
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

func seedShortPeriodConnector(t *testing.T, ctx context.Context, st store.Store, payload string, testPeriodSeconds int) int64 {
	t.Helper()

	err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  payload,
		Name:          "test-short-connector",
		ChatID:        "1003626584986",
		PriceRUB:      2322,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: int64(testPeriodSeconds),
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("create short connector: %v", err)
	}

	connector, found, err := st.GetConnectorByStartPayload(ctx, payload)
	if err != nil {
		t.Fatalf("get short connector by payload: %v", err)
	}
	if !found {
		t.Fatalf("short connector not found by payload=%s", payload)
	}
	return connector.ID
}

func seedPayment(t *testing.T, ctx context.Context, st store.Store, payment domain.Payment) {
	t.Helper()
	if payment.UserID <= 0 {
		t.Fatalf("seed payment requires user_id")
	}
	if err := st.CreatePayment(ctx, payment); err != nil {
		t.Fatalf("create payment: %v", err)
	}
}

func seedTelegramUser(t *testing.T, ctx context.Context, st store.Store, telegramID int64) int64 {
	t.Helper()
	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, fmt.Sprintf("%d", telegramID), "")
	if err != nil {
		t.Fatalf("get or create telegram user: %v", err)
	}
	return user.ID
}

func requireTelegramUserID(t *testing.T, ctx context.Context, st store.Store, userID int64) int64 {
	t.Helper()
	accounts, err := st.ListUserMessengerAccounts(ctx, userID)
	if err != nil {
		t.Fatalf("list user messenger accounts: %v", err)
	}
	for _, account := range accounts {
		if account.MessengerKind != domain.MessengerKindTelegram {
			continue
		}
		telegramID, parseErr := strconv.ParseInt(account.MessengerUserID, 10, 64)
		if parseErr != nil || telegramID <= 0 {
			t.Fatalf("invalid telegram messenger user id %q", account.MessengerUserID)
		}
		return telegramID
	}
	t.Fatalf("telegram account not found for user_id=%d", userID)
	return 0
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
