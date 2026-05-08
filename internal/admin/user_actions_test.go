package admin

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

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
)

type adminSpySender struct {
	sent []adminSentMessage
}

type adminSentMessage struct {
	user messenger.UserRef
	msg  messenger.OutgoingMessage
}

func (s *adminSpySender) Send(_ context.Context, user messenger.UserRef, msg messenger.OutgoingMessage) error {
	s.sent = append(s.sent, adminSentMessage{user: user, msg: msg})
	return nil
}

func (s *adminSpySender) Edit(context.Context, messenger.MessageRef, messenger.OutgoingMessage) error {
	return nil
}

func (s *adminSpySender) AnswerAction(context.Context, messenger.ActionRef, string) error {
	return nil
}

func TestSendUserMessage_AllowsUserIDResolution(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("telegram.NewClient: %v", err)
	}
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", tg, nil, nil)

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "264704572", "emiloserdov")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	user.FullName = "Egor Miloserdov"
	user.UpdatedAt = time.Now().UTC()
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	user, found, err := st.GetUser(ctx, 264704572)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if !found {
		t.Fatal("expected saved user")
	}

	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/users/view?lang=ru&user_id="+strconv.FormatInt(user.ID, 10), nil)
	csrfRec := httptest.NewRecorder()
	csrfToken := h.ensureCSRFToken(csrfRec, csrfReq)
	csrfResp := csrfRec.Result()
	defer csrfResp.Body.Close()

	var csrfCookie *http.Cookie
	for _, c := range csrfResp.Cookies() {
		if c.Name == csrfCookieName {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("csrf cookie missing")
	}

	form := url.Values{}
	form.Set("csrf_token", csrfToken)
	form.Set("user_id", strconv.FormatInt(user.ID, 10))
	form.Set("message", "test message from admin")

	req := httptest.NewRequest(http.MethodPost, "/admin/users/message?lang=ru", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	h.sendUserMessage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "сообщение отправлено пользователю") {
		t.Fatalf("response does not contain success notice: %q", rec.Body.String())
	}
}

func TestSendUserAccessLink_CreatesStoresAndSendsTelegramInvite(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Now().UTC()
	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "264704572", "emiloserdov")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-admin-access-link",
		Name:          "Paid Chat",
		ChatID:        "1001234567890",
		PriceRUB:      1000,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: int64((30 * 24 * time.Hour).Seconds()),
		IsActive:      true,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-admin-access-link")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPaid,
		Token:       "paid-admin-access-link",
		UserID:      user.ID,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	payment, found, err := st.GetPaymentByToken(ctx, "paid-admin-access-link")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         user.ID,
		ConnectorID:    connector.ID,
		PaymentID:      payment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(-time.Hour),
		EndsAt:         now.Add(10 * time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment: %v", err)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, user.ID, connector.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}

	var createInvitePayload map[string]any
	sendCount := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/createChatInviteLink"):
			if err := r.ParseMultipartForm(1024 * 1024); err != nil {
				body, _ := io.ReadAll(r.Body)
				t.Fatalf("decode create invite payload: %v body=%q", err, string(body))
			}
			createInvitePayload = map[string]any{
				"chat_id": r.FormValue("chat_id"),
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":{"invite_link":"https://t.me/+fresh-admin-link","creator":{"id":1,"is_bot":true,"first_name":"bot"},"creates_join_request":false,"is_primary":false,"is_revoked":false}}`))
		case strings.Contains(r.URL.Path, "/sendMessage"):
			sendCount++
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":1770000000,"chat":{"id":264704572,"type":"private"},"text":"ok"}}`))
		default:
			t.Fatalf("unexpected telegram api path: %s", r.URL.Path)
		}
	}))
	defer api.Close()
	tg, err := telegram.NewClientWithOptions("123:test", "", telegram.ClientOptions{ServerURL: api.URL})
	if err != nil {
		t.Fatalf("NewClientWithOptions: %v", err)
	}
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", tg, nil, nil)

	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/users/view?lang=ru&user_id="+strconv.FormatInt(user.ID, 10), nil)
	csrfRec := httptest.NewRecorder()
	csrfToken := h.ensureCSRFToken(csrfRec, csrfReq)
	csrfResp := csrfRec.Result()
	defer csrfResp.Body.Close()
	csrfCookie := csrfResp.Cookies()[0]

	form := url.Values{}
	form.Set("csrf_token", csrfToken)
	form.Set("user_id", strconv.FormatInt(user.ID, 10))
	form.Set("subscription_id", strconv.FormatInt(sub.ID, 10))
	req := httptest.NewRequest(http.MethodPost, "/admin/users/send-access-link?lang=ru", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	h.sendUserAccessLink(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rec.Code, rec.Body.String())
	}
	if createInvitePayload["chat_id"] != "-1001234567890" {
		t.Fatalf("chat_id payload=%v want -1001234567890", createInvitePayload["chat_id"])
	}
	if sendCount != 1 {
		t.Fatalf("sendMessage calls=%d want 1", sendCount)
	}
	links, err := st.ListActiveTelegramInviteLinks(ctx, user.ID, "-1001234567890", now)
	if err != nil {
		t.Fatalf("ListActiveTelegramInviteLinks: %v", err)
	}
	if len(links) != 1 || links[0].InviteLink != "https://t.me/+fresh-admin-link" {
		t.Fatalf("links=%+v want saved fresh link", links)
	}
}

func TestUnbanUserAndSendAccessLink_UnbansThenSendsFreshInvite(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Now().UTC()
	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "264704572", "emiloserdov")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-admin-unban-access-link",
		Name:          "Paid Chat",
		ChatID:        "1001234567890",
		PriceRUB:      1000,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: int64((30 * 24 * time.Hour).Seconds()),
		IsActive:      true,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-admin-unban-access-link")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPaid,
		Token:       "paid-admin-unban-access-link",
		UserID:      user.ID,
		ConnectorID: connector.ID,
		AmountRUB:   1000,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	payment, found, err := st.GetPaymentByToken(ctx, "paid-admin-unban-access-link")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         user.ID,
		ConnectorID:    connector.ID,
		PaymentID:      payment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(-time.Hour),
		EndsAt:         now.Add(10 * time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment: %v", err)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, user.ID, connector.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}

	var (
		calls        []string
		unbanPayload map[string]string
		sendCount    int
	)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			body, _ := io.ReadAll(r.Body)
			t.Fatalf("decode telegram payload for %s: %v body=%q", r.URL.Path, err, string(body))
		}
		switch {
		case strings.Contains(r.URL.Path, "/unbanChatMember"):
			calls = append(calls, "unban")
			unbanPayload = map[string]string{
				"chat_id":        r.FormValue("chat_id"),
				"user_id":        r.FormValue("user_id"),
				"only_if_banned": r.FormValue("only_if_banned"),
			}
			_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
		case strings.Contains(r.URL.Path, "/createChatInviteLink"):
			calls = append(calls, "create")
			_, _ = w.Write([]byte(`{"ok":true,"result":{"invite_link":"https://t.me/+fresh-after-unban","creator":{"id":1,"is_bot":true,"first_name":"bot"},"creates_join_request":false,"is_primary":false,"is_revoked":false}}`))
		case strings.Contains(r.URL.Path, "/sendMessage"):
			calls = append(calls, "send")
			sendCount++
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"date":1770000000,"chat":{"id":264704572,"type":"private"},"text":"ok"}}`))
		default:
			t.Fatalf("unexpected telegram api path: %s", r.URL.Path)
		}
	}))
	defer api.Close()
	tg, err := telegram.NewClientWithOptions("123:test", "", telegram.ClientOptions{ServerURL: api.URL})
	if err != nil {
		t.Fatalf("NewClientWithOptions: %v", err)
	}
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", tg, nil, nil)

	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/users/view?lang=ru&user_id="+strconv.FormatInt(user.ID, 10), nil)
	csrfRec := httptest.NewRecorder()
	csrfToken := h.ensureCSRFToken(csrfRec, csrfReq)
	csrfResp := csrfRec.Result()
	defer csrfResp.Body.Close()
	csrfCookie := csrfResp.Cookies()[0]

	form := url.Values{}
	form.Set("csrf_token", csrfToken)
	form.Set("user_id", strconv.FormatInt(user.ID, 10))
	form.Set("subscription_id", strconv.FormatInt(sub.ID, 10))
	req := httptest.NewRequest(http.MethodPost, "/admin/users/unban-and-send-access-link?lang=ru", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	h.unbanUserAndSendAccessLink(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rec.Code, rec.Body.String())
	}
	wantCalls := strings.Join([]string{"unban", "create", "send"}, ",")
	if got := strings.Join(calls, ","); got != wantCalls {
		t.Fatalf("telegram calls=%s want %s", got, wantCalls)
	}
	if unbanPayload["chat_id"] != "-1001234567890" {
		t.Fatalf("unban chat_id=%q want -1001234567890", unbanPayload["chat_id"])
	}
	if unbanPayload["user_id"] != "264704572" {
		t.Fatalf("unban user_id=%q want 264704572", unbanPayload["user_id"])
	}
	if unbanPayload["only_if_banned"] != "true" {
		t.Fatalf("unban only_if_banned=%q want true", unbanPayload["only_if_banned"])
	}
	if sendCount != 1 {
		t.Fatalf("sendMessage calls=%d want 1", sendCount)
	}
	links, err := st.ListActiveTelegramInviteLinks(ctx, user.ID, "-1001234567890", now)
	if err != nil {
		t.Fatalf("ListActiveTelegramInviteLinks: %v", err)
	}
	if len(links) != 1 || links[0].InviteLink != "https://t.me/+fresh-after-unban" {
		t.Fatalf("links=%+v want saved fresh link", links)
	}
}

func TestSendUserMessage_DeliversToMAXOnlyUser(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	maxSpy := &adminSpySender{}
	h := NewHandler(st, "test-admin-token", "test_bot", "id9718272494_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, maxSpy, nil)

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "fedor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	user.FullName = "Fedor"
	user.UpdatedAt = time.Now().UTC()
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/users/view?lang=ru&user_id="+strconv.FormatInt(user.ID, 10), nil)
	csrfRec := httptest.NewRecorder()
	csrfToken := h.ensureCSRFToken(csrfRec, csrfReq)
	csrfResp := csrfRec.Result()
	defer csrfResp.Body.Close()

	var csrfCookie *http.Cookie
	for _, c := range csrfResp.Cookies() {
		if c.Name == csrfCookieName {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("csrf cookie missing")
	}

	form := url.Values{}
	form.Set("csrf_token", csrfToken)
	form.Set("user_id", strconv.FormatInt(user.ID, 10))
	form.Set("message", "test message to max")

	req := httptest.NewRequest(http.MethodPost, "/admin/users/message?lang=ru", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	h.sendUserMessage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(maxSpy.sent) != 1 {
		t.Fatalf("max messages sent = %d, want 1", len(maxSpy.sent))
	}
	if maxSpy.sent[0].user.Kind != messenger.KindMAX {
		t.Fatalf("kind = %s, want %s", maxSpy.sent[0].user.Kind, messenger.KindMAX)
	}
	if maxSpy.sent[0].user.UserID != 193465776 {
		t.Fatalf("user id = %d, want 193465776", maxSpy.sent[0].user.UserID)
	}
	if maxSpy.sent[0].msg.Text != "test message to max" {
		t.Fatalf("text = %q, want direct MAX message", maxSpy.sent[0].msg.Text)
	}
}

func TestSendUserPaymentLink_DeliversToMAXOnlyUser(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	maxSpy := &adminSpySender{}
	h := NewHandler(st, "test-admin-token", "test_bot", "id9718272494_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, maxSpy, nil)

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "fedor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-paylink-max",
		Name:          "MAX tariff",
		PriceRUB:      500,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-paylink-max")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPaid,
		Token:       "max-paylink-payment",
		UserID:      user.ID,
		ConnectorID: connector.ID,
		AmountRUB:   connector.PriceRUB,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	paymentRow, found, err := st.GetPaymentByToken(ctx, "max-paylink-payment")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:      user.ID,
		ConnectorID: connector.ID,
		PaymentID:   paymentRow.ID,
		Status:      domain.SubscriptionStatusActive,
		StartsAt:    time.Now().UTC(),
		EndsAt:      time.Now().UTC().Add(30 * 24 * time.Hour),
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment: %v", err)
	}
	subscription, found, err := st.GetLatestSubscriptionByUserConnector(ctx, user.ID, connector.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}

	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/users/view?lang=ru&user_id="+strconv.FormatInt(user.ID, 10), nil)
	csrfRec := httptest.NewRecorder()
	csrfToken := h.ensureCSRFToken(csrfRec, csrfReq)
	csrfResp := csrfRec.Result()
	defer csrfResp.Body.Close()

	var csrfCookie *http.Cookie
	for _, c := range csrfResp.Cookies() {
		if c.Name == csrfCookieName {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("csrf cookie missing")
	}

	form := url.Values{}
	form.Set("csrf_token", csrfToken)
	form.Set("user_id", strconv.FormatInt(user.ID, 10))
	form.Set("subscription_id", strconv.FormatInt(subscription.ID, 10))

	req := httptest.NewRequest(http.MethodPost, "/admin/users/send-payment-link?lang=ru", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	h.sendUserPaymentLink(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(maxSpy.sent) != 1 {
		t.Fatalf("max payment-link messages sent = %d, want 1", len(maxSpy.sent))
	}
	msg := maxSpy.sent[0].msg
	if maxSpy.sent[0].user.Kind != messenger.KindMAX {
		t.Fatalf("kind = %s, want %s", maxSpy.sent[0].user.Kind, messenger.KindMAX)
	}
	if !strings.Contains(msg.Text, "/start in-paylink-max") {
		t.Fatalf("text = %q, want MAX fallback command", msg.Text)
	}
	if len(msg.Buttons) != 1 || len(msg.Buttons[0]) != 1 {
		t.Fatalf("buttons = %#v, want single deeplink button", msg.Buttons)
	}
	if got := msg.Buttons[0][0].URL; got != "https://max.ru/id9718272494_bot?start=in-paylink-max" {
		t.Fatalf("button url = %q, want MAX deeplink", got)
	}
}
