package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/chatstatus"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/max"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
)

type adminFakeTelegramChecker struct {
	chatRef string
	userID  int64
	info    telegram.ChatMemberInfo
}

func (f *adminFakeTelegramChecker) GetChatMember(_ context.Context, chatRef string, userID int64) (telegram.ChatMemberInfo, error) {
	f.chatRef = chatRef
	f.userID = userID
	return f.info, nil
}

type adminFakeMAXChecker struct {
	chatID  int64
	userIDs []int64
	page    max.ChatMembersPage
}

func (f *adminFakeMAXChecker) GetChatMembers(_ context.Context, chatID int64, userIDs []int64) (max.ChatMembersPage, error) {
	f.chatID = chatID
	f.userIDs = append([]int64(nil), userIDs...)
	return f.page, nil
}

func TestUserDetailPage_ShowsChatAccessStatusCards(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "id9718272494_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)
	now := time.Now().UTC()

	user, sub := createAdminChatStatusFixture(t, ctx, st, domain.MessengerKindTelegram, "464152205", domain.Connector{
		StartPayload: "in-chat-status-card",
		Name:         "Private Telegram",
		ChatID:       "1003626584986",
		PriceRUB:     5000,
		PeriodMode:   domain.ConnectorPeriodModeCalendarMonths,
		PeriodMonths: 1,
		IsActive:     true,
		CreatedAt:    now,
	})
	if err := st.UpsertMessengerChatUser(ctx, domain.MessengerChatUser{
		MessengerKind:   domain.MessengerKindTelegram,
		ChatRef:         "-1003626584986",
		MessengerUserID: "464152205",
		UserID:          user.ID,
		Status:          domain.MessengerChatUserStatusBanned,
		IsBanned:        true,
		CheckedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatalf("UpsertMessengerChatUser: %v", err)
	}
	_ = sub

	req := httptest.NewRequest(http.MethodGet, "/admin/users/view?lang=ru&user_id="+strconv.FormatInt(user.ID, 10), nil)
	rec := httptest.NewRecorder()
	h.userDetailPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Доступ в чаты", "Private Telegram", "-1003626584986", "заблокирован"} {
		if !strings.Contains(body, want) {
			t.Fatalf("response does not contain %q: %q", want, body)
		}
	}
}

func TestRefreshUserChatStatus_StoresTelegramBanStatus(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	user, sub := createAdminChatStatusFixture(t, ctx, st, domain.MessengerKindTelegram, "464152205", domain.Connector{
		StartPayload: "in-chat-status-refresh-tg",
		Name:         "Private Telegram",
		ChatID:       "1003626584986",
		PriceRUB:     5000,
		PeriodMode:   domain.ConnectorPeriodModeCalendarMonths,
		PeriodMonths: 1,
		IsActive:     true,
		CreatedAt:    now,
	})
	checker := &adminFakeTelegramChecker{info: telegram.ChatMemberInfo{Status: "kicked", Raw: []byte(`{"status":"kicked"}`)}}
	h := NewHandler(st, "test-admin-token", "test_bot", "id9718272494_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)
	h.SetChatStatusService(chatstatus.Service{Store: st, Telegram: checker, Now: func() time.Time { return now }})

	postAdminChatStatusRefresh(t, h, user.ID, sub.ID, domain.MessengerKindTelegram)

	if checker.chatRef != "-1003626584986" || checker.userID != 464152205 {
		t.Fatalf("telegram checker target chat=%q user=%d", checker.chatRef, checker.userID)
	}
	state, found, err := st.GetMessengerChatUser(ctx, domain.MessengerKindTelegram, "-1003626584986", "464152205")
	if err != nil || !found {
		t.Fatalf("GetMessengerChatUser found=%v err=%v", found, err)
	}
	if state.Status != domain.MessengerChatUserStatusBanned || !state.IsBanned {
		t.Fatalf("state=%+v want banned", state)
	}
	checks, err := st.ListMessengerChatUserChecks(ctx, domain.MessengerKindTelegram, "-1003626584986", "464152205", 10)
	if err != nil {
		t.Fatalf("ListMessengerChatUserChecks: %v", err)
	}
	if len(checks) != 1 || checks[0].Source != "admin_user_card_refresh" || string(checks[0].RawJSON) != `{"status":"kicked"}` {
		t.Fatalf("checks=%+v want stored admin raw check", checks)
	}
}

func TestRefreshUserChatStatus_StoresMAXMemberStatus(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	now := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	user, sub := createAdminChatStatusFixture(t, ctx, st, domain.MessengerKindMAX, "193465776", domain.Connector{
		StartPayload:  "in-chat-status-refresh-max",
		Name:          "MAX Chat",
		MAXChatID:     "-73770240324272",
		MAXChannelURL: "https://web.max.ru/-73770240324272",
		PriceRUB:      5000,
		PeriodMode:    domain.ConnectorPeriodModeCalendarMonths,
		PeriodMonths:  1,
		IsActive:      true,
		CreatedAt:     now,
	})
	checker := &adminFakeMAXChecker{page: max.ChatMembersPage{
		Members: []max.ChatMember{{UserID: 193465776, IsAdmin: true, Permissions: []string{"read_all_messages"}}},
		Raw:     []byte(`{"members":[{"user_id":193465776,"is_admin":true}]}`),
	}}
	h := NewHandler(st, "test-admin-token", "test_bot", "id9718272494_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)
	h.SetChatStatusService(chatstatus.Service{Store: st, MAX: checker, Now: func() time.Time { return now }})

	postAdminChatStatusRefresh(t, h, user.ID, sub.ID, domain.MessengerKindMAX)

	if checker.chatID != -73770240324272 || len(checker.userIDs) != 1 || checker.userIDs[0] != 193465776 {
		t.Fatalf("max checker target chat=%d users=%v", checker.chatID, checker.userIDs)
	}
	state, found, err := st.GetMessengerChatUser(ctx, domain.MessengerKindMAX, "-73770240324272", "193465776")
	if err != nil || !found {
		t.Fatalf("GetMessengerChatUser found=%v err=%v", found, err)
	}
	if state.Status != domain.MessengerChatUserStatusAdministrator || !state.IsAdmin || !state.IsMember {
		t.Fatalf("state=%+v want max administrator", state)
	}
}

func createAdminChatStatusFixture(t *testing.T, ctx context.Context, st *memory.Store, kind domain.MessengerKind, messengerUserID string, connector domain.Connector) (domain.User, domain.Subscription) {
	t.Helper()
	now := time.Now().UTC()
	user, _, err := st.GetOrCreateUserByMessenger(ctx, kind, messengerUserID, "testuser")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	if connector.CreatedAt.IsZero() {
		connector.CreatedAt = now
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connectorRow, found, err := st.GetConnectorByStartPayload(ctx, connector.StartPayload)
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPaid,
		Token:       connector.StartPayload + "-payment",
		UserID:      user.ID,
		ConnectorID: connectorRow.ID,
		AmountRUB:   connectorRow.PriceRUB,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	payment, found, err := st.GetPaymentByToken(ctx, connector.StartPayload+"-payment")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	sub := domain.Subscription{
		UserID:      user.ID,
		ConnectorID: connectorRow.ID,
		PaymentID:   payment.ID,
		Status:      domain.SubscriptionStatusActive,
		StartsAt:    now.Add(-time.Hour),
		EndsAt:      now.Add(24 * time.Hour),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.UpsertSubscriptionByPayment(ctx, sub); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment: %v", err)
	}
	subscription, found, err := st.GetLatestSubscriptionByUserConnector(ctx, user.ID, connectorRow.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}
	return user, subscription
}

func postAdminChatStatusRefresh(t *testing.T, h *Handler, userID, subscriptionID int64, kind domain.MessengerKind) {
	t.Helper()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/users/view?lang=ru&user_id="+strconv.FormatInt(userID, 10), nil)
	csrfRec := httptest.NewRecorder()
	csrfToken := h.ensureCSRFToken(csrfRec, csrfReq)
	csrfResp := csrfRec.Result()
	defer csrfResp.Body.Close()
	csrfCookie := csrfResp.Cookies()[0]

	form := url.Values{}
	form.Set("csrf_token", csrfToken)
	form.Set("user_id", strconv.FormatInt(userID, 10))
	form.Set("subscription_id", strconv.FormatInt(subscriptionID, 10))
	form.Set("messenger_kind", string(kind))

	req := httptest.NewRequest(http.MethodPost, "/admin/users/refresh-chat-status?lang=ru", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	h.refreshUserChatStatus(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "статус доступа в чате обновлен") {
		t.Fatalf("response does not contain success notice: %q", rec.Body.String())
	}
}
