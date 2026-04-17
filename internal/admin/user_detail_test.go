package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestUserDetailPage_ShowsMAXComposeHelperForMAXUser(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "id9718272494_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "fedor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	user.FullName = "Fedor"
	user.UpdatedAt = time.Now().UTC()
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/users/view?lang=ru&user_id="+strconv.FormatInt(user.ID, 10), nil)
	rec := httptest.NewRecorder()
	h.userDetailPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Открыть чат в MAX") {
		t.Fatalf("response does not contain MAX compose action: %q", body)
	}
	if !strings.Contains(body, "https://max.ru/id9718272494_bot") {
		t.Fatalf("response does not contain MAX bot chat url: %q", body)
	}
	if !strings.Contains(body, "Скопировать текст и открыть") {
		t.Fatalf("response does not contain copy+open helper: %q", body)
	}
}

func TestUserDetailPage_DoesNotShowMAXComposeHelperForTelegramUser(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "id9718272494_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "264704572", "emiloserdov")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	user.FullName = "Egor"
	user.UpdatedAt = time.Now().UTC()
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/users/view?lang=ru&user_id="+strconv.FormatInt(user.ID, 10), nil)
	rec := httptest.NewRecorder()
	h.userDetailPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "Открыть чат в MAX") {
		t.Fatalf("response unexpectedly contains MAX compose action: %q", body)
	}
}

func TestUserDetailPage_ShowsFutureRenewalAsNextPeriodWithoutSecondRevokeAction(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "id9718272494_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)
	now := time.Now().UTC()

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "264704573", "egor2")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-user-detail-next-period",
		Name:          "Next period tariff",
		ChatID:        "1003626584986",
		PriceRUB:      500,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 6 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-user-detail-next-period")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}

	for idx, token := range []string{"user-detail-current", "user-detail-future"} {
		if err := st.CreatePayment(ctx, domain.Payment{
			Provider:       "robokassa",
			Status:         domain.PaymentStatusPaid,
			Token:          token,
			UserID:         user.ID,
			ConnectorID:    connector.ID,
			AmountRUB:      connector.PriceRUB,
			AutoPayEnabled: true,
			CreatedAt:      now.Add(time.Duration(idx) * time.Minute),
			UpdatedAt:      now.Add(time.Duration(idx) * time.Minute),
		}); err != nil {
			t.Fatalf("CreatePayment(%s): %v", token, err)
		}
	}
	currentPayment, found, err := st.GetPaymentByToken(ctx, "user-detail-current")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken current found=%v err=%v", found, err)
	}
	futurePayment, found, err := st.GetPaymentByToken(ctx, "user-detail-future")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken future found=%v err=%v", found, err)
	}

	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         user.ID,
		ConnectorID:    connector.ID,
		PaymentID:      currentPayment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(-time.Hour),
		EndsAt:         now.Add(time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment current: %v", err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         user.ID,
		ConnectorID:    connector.ID,
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

	req := httptest.NewRequest(http.MethodGet, "/admin/users/view?lang=ru&user_id="+strconv.FormatInt(user.ID, 10), nil)
	rec := httptest.NewRecorder()
	h.userDetailPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Всего периодов") || !strings.Contains(body, "Текущие периоды") || !strings.Contains(body, "Следующие периоды") {
		t.Fatalf("response does not contain phase summary cards: %q", body)
	}
	if !strings.Contains(body, "Периоды доступа") {
		t.Fatalf("response does not contain updated access periods title: %q", body)
	}
	if !strings.Contains(body, "текущий период") {
		t.Fatalf("response does not contain current period badge: %q", body)
	}
	if !strings.Contains(body, "следующий период") {
		t.Fatalf("response does not contain future renewal badge: %q", body)
	}
	currentPos := strings.Index(body, ">"+strconv.FormatInt(currentPayment.ID, 10)+"<")
	futurePos := strings.Index(body, ">"+strconv.FormatInt(futurePayment.ID, 10)+"<")
	if currentPos == -1 || futurePos == -1 {
		t.Fatalf("response does not contain expected payment ids: %q", body)
	}
	if currentPos > futurePos {
		t.Fatalf("current subscription rendered after future renewal: current=%d future=%d body=%q", currentPos, futurePos, body)
	}
	if got := strings.Count(body, "/admin/subscriptions/revoke?"); got != 1 {
		t.Fatalf("revoke action count = %d, want 1 current subscription only", got)
	}
}
