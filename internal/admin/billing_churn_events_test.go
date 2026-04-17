package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestBillingPage_ShowsPrimaryMessengerInsteadOfTelegramID(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "fedor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	user.FullName = "Fedor"
	user.UpdatedAt = time.Now().UTC()
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	connector := domain.Connector{
		StartPayload:  "in-billing-max",
		Name:          "MAX tariff",
		PriceRUB:      500,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connectorRow, found, err := st.GetConnectorByStartPayload(ctx, "in-billing-max")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPaid,
		Token:       "billing-max-payment",
		UserID:      user.ID,
		ConnectorID: connectorRow.ID,
		AmountRUB:   connectorRow.PriceRUB,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	payment, found, err := st.GetPaymentByToken(ctx, "billing-max-payment")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         user.ID,
		ConnectorID:    connectorRow.ID,
		PaymentID:      payment.ID,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       time.Now().UTC(),
		EndsAt:         time.Now().UTC().Add(30 * 24 * time.Hour),
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/billing?lang=ru&user_id=1", nil)
	rec := httptest.NewRecorder()
	h.billingPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Основной мессенджер") {
		t.Fatalf("response does not contain primary messenger header: %q", body)
	}
	if !strings.Contains(body, "MAX · 193465776 · @fedor") {
		t.Fatalf("response does not contain MAX primary account: %q", body)
	}
	if strings.Contains(body, ">Telegram ID<") {
		t.Fatalf("response still contains Telegram-first column: %q", body)
	}
}

func TestBuildBillingSummary_CountsOnlyCurrentActiveSubscriptions(t *testing.T) {
	now := time.Now().UTC()
	summary, groups := buildBillingSummary(nil, []domain.Subscription{
		{
			ID:          1,
			ConnectorID: 10,
			Status:      domain.SubscriptionStatusActive,
			StartsAt:    now.Add(-time.Hour),
			EndsAt:      now.Add(time.Hour),
		},
		{
			ID:          2,
			ConnectorID: 10,
			Status:      domain.SubscriptionStatusActive,
			StartsAt:    now.Add(time.Hour),
			EndsAt:      now.Add(2 * time.Hour),
		},
	}, map[int64]string{10: "Test"}, now)

	if summary.ActiveSubscriptions != 1 {
		t.Fatalf("summary.ActiveSubscriptions = %d, want 1", summary.ActiveSubscriptions)
	}
	if summary.NextSubscriptions != 1 {
		t.Fatalf("summary.NextSubscriptions = %d, want 1", summary.NextSubscriptions)
	}
	if len(groups) != 1 || groups[0].ActiveSubscriptions != 1 || groups[0].NextSubscriptions != 1 {
		t.Fatalf("groups = %+v, want one current and one next subscription", groups)
	}
}

func TestSubscriptionStatusBadgeAt_FutureActiveUsesNextPeriodLabel(t *testing.T) {
	now := time.Now().UTC()
	label, className := subscriptionStatusBadgeAt("ru", domain.Subscription{
		Status:   domain.SubscriptionStatusActive,
		StartsAt: now.Add(time.Hour),
		EndsAt:   now.Add(2 * time.Hour),
	}, now)
	if label != "следующий период" || className != "is-accent" {
		t.Fatalf("subscriptionStatusBadgeAt() = (%q,%q), want (следующий период,is-accent)", label, className)
	}
}

func TestSubscriptionStatusBadgeAt_CurrentActiveUsesCurrentPeriodLabel(t *testing.T) {
	now := time.Now().UTC()
	label, className := subscriptionStatusBadgeAt("ru", domain.Subscription{
		Status:   domain.SubscriptionStatusActive,
		StartsAt: now.Add(-time.Hour),
		EndsAt:   now.Add(time.Hour),
	}, now)
	if label != "текущий период" || className != "is-success" {
		t.Fatalf("subscriptionStatusBadgeAt() = (%q,%q), want (текущий период,is-success)", label, className)
	}
}

func TestBillingPage_SortsCurrentSubscriptionBeforeFutureRenewal(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)
	now := time.Now().UTC()

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "264704572", "egor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-billing-phase-order",
		Name:          "Phase ordering",
		ChatID:        "1003626584986",
		PriceRUB:      500,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-billing-phase-order")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}

	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		ID:          401,
		UserID:      user.ID,
		ConnectorID: connector.ID,
		PaymentID:   501,
		Status:      domain.SubscriptionStatusActive,
		StartsAt:    now.Add(time.Hour),
		EndsAt:      now.Add(2 * time.Hour),
		CreatedAt:   now.Add(-time.Minute),
		UpdatedAt:   now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment future: %v", err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		ID:          402,
		UserID:      user.ID,
		ConnectorID: connector.ID,
		PaymentID:   502,
		Status:      domain.SubscriptionStatusActive,
		StartsAt:    now.Add(-time.Hour),
		EndsAt:      now.Add(time.Hour),
		CreatedAt:   now.Add(-2 * time.Minute),
		UpdatedAt:   now.Add(-2 * time.Minute),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment current: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/billing?lang=ru&user_id=1", nil)
	rec := httptest.NewRecorder()
	h.billingPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "текущий период") || !strings.Contains(body, "следующий период") {
		t.Fatalf("response does not contain phase labels: %q", body)
	}
	if !strings.Contains(body, "Текущих периодов") || !strings.Contains(body, "Следующих периодов") {
		t.Fatalf("response does not contain phase summary labels: %q", body)
	}
	if !strings.Contains(body, "Периоды доступа") || !strings.Contains(body, "Текущие периоды") {
		t.Fatalf("response does not contain updated phase vocabulary: %q", body)
	}
	currentPos := strings.Index(body, ">502<")
	futurePos := strings.Index(body, ">501<")
	if currentPos == -1 || futurePos == -1 {
		t.Fatalf("response does not contain expected payment ids: %q", body)
	}
	if currentPos > futurePos {
		t.Fatalf("current subscription rendered after future renewal: current=%d future=%d body=%q", currentPos, futurePos, body)
	}
}

func TestChurnPage_ShowsMessengerNeutralUserSummary(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "fedor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	user.FullName = "Fedor"
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	connector := domain.Connector{
		StartPayload:  "in-churn-max",
		Name:          "MAX churn",
		PriceRUB:      200,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connectorRow, found, err := st.GetConnectorByStartPayload(ctx, "in-churn-max")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusFailed,
		Token:       "churn-max-payment",
		UserID:      user.ID,
		ConnectorID: connectorRow.ID,
		AmountRUB:   connectorRow.PriceRUB,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/churn?lang=ru&user_id=1", nil)
	rec := httptest.NewRecorder()
	h.churnPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "MAX · 193465776 · @fedor") {
		t.Fatalf("response does not contain MAX account summary: %q", body)
	}
	if strings.Contains(body, "telegram_id=") {
		t.Fatalf("response still contains telegram_id marker: %q", body)
	}
}

func TestEventsPage_FiltersAndRendersMAXTargetAccount(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	if err := st.SaveAuditEvent(ctx, domain.AuditEvent{
		ActorType:             domain.AuditActorTypeApp,
		TargetMessengerKind:   domain.MessengerKindMAX,
		TargetMessengerUserID: "193465776",
		Action:                "max_action",
		Details:               "max target",
		CreatedAt:             time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveAuditEvent max: %v", err)
	}
	if err := st.SaveAuditEvent(ctx, domain.AuditEvent{
		ActorType:             domain.AuditActorTypeApp,
		TargetMessengerKind:   domain.MessengerKindTelegram,
		TargetMessengerUserID: "264704572",
		Action:                "telegram_action",
		Details:               "telegram target",
		CreatedAt:             time.Now().UTC().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SaveAuditEvent telegram: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/events?lang=ru&messenger_kind=max&messenger_user_id=193465776", nil)
	rec := httptest.NewRecorder()
	h.eventsPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "MAX · 193465776") {
		t.Fatalf("response does not contain MAX target account: %q", body)
	}
	if !strings.Contains(body, "max_action") {
		t.Fatalf("response does not contain filtered MAX event: %q", body)
	}
	if strings.Contains(body, "telegram_action") {
		t.Fatalf("response still contains telegram event after MAX filter: %q", body)
	}
}
