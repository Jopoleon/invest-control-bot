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

func TestBuildPaymentAccessStatus_UsesLatestAuditEvent(t *testing.T) {
	payment := domain.Payment{ID: 17, Status: domain.PaymentStatusPaid}
	events := []domain.AuditEvent{
		{Action: domain.AuditActionAccessDeliveryFailed, Details: "payment_id=17", CreatedAt: time.Now().UTC()},
		{Action: domain.AuditActionPaymentAccessReady, Details: "payment_id=17", CreatedAt: time.Now().UTC().Add(-time.Minute)},
	}

	label, className := buildPaymentAccessStatus("ru", payment, events)
	if label != "проблема доступа" {
		t.Fatalf("label=%q want problem badge", label)
	}
	if className != "is-danger" {
		t.Fatalf("class=%q want is-danger", className)
	}
}

func TestBuildSubscriptionAccessStatus_PrefersManualCheckForExpiredSubscription(t *testing.T) {
	sub := domain.Subscription{ID: 29, Status: domain.SubscriptionStatusExpired}
	events := []domain.AuditEvent{
		{Action: domain.AuditActionSubscriptionRevokeManualCheck, Details: "subscription_id=29", CreatedAt: time.Now().UTC()},
		{Action: domain.AuditActionSubscriptionRevokeFailed, Details: "subscription_id=29", CreatedAt: time.Now().UTC().Add(-time.Minute)},
	}

	label, className := buildSubscriptionAccessStatus("ru", sub, events)
	if label != "нужна ручная проверка" {
		t.Fatalf("label=%q want manual-check badge", label)
	}
	if className != "is-danger" {
		t.Fatalf("class=%q want is-danger", className)
	}
}

func TestBillingPage_RendersAccessAndRevokeOperationalStatuses(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "264704572", "egor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	connector := domain.Connector{
		StartPayload:  "in-billing-access-ops",
		Name:          "TG tariff",
		ChatID:        "1003626584986",
		PriceRUB:      500,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}
	if err := st.CreateConnector(ctx, connector); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connectorRow, found, err := st.GetConnectorByStartPayload(ctx, "in-billing-access-ops")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:    "robokassa",
		Status:      domain.PaymentStatusPaid,
		Token:       "billing-access-payment",
		UserID:      user.ID,
		ConnectorID: connectorRow.ID,
		AmountRUB:   connectorRow.PriceRUB,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	payment, found, err := st.GetPaymentByToken(ctx, "billing-access-payment")
	if err != nil || !found {
		t.Fatalf("GetPaymentByToken found=%v err=%v", found, err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         user.ID,
		ConnectorID:    connectorRow.ID,
		PaymentID:      payment.ID,
		Status:         domain.SubscriptionStatusExpired,
		AutoPayEnabled: true,
		StartsAt:       time.Now().UTC().Add(-48 * time.Hour),
		EndsAt:         time.Now().UTC().Add(-24 * time.Hour),
		CreatedAt:      time.Now().UTC().Add(-48 * time.Hour),
		UpdatedAt:      time.Now().UTC().Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment: %v", err)
	}
	sub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, user.ID, connectorRow.ID)
	if err != nil || !found {
		t.Fatalf("GetLatestSubscriptionByUserConnector found=%v err=%v", found, err)
	}
	if err := st.SaveAuditEvent(ctx, domain.AuditEvent{
		TargetUserID: user.ID,
		ConnectorID:  connectorRow.ID,
		Action:       domain.AuditActionPaymentAccessReady,
		Details:      "payment_id=" + strconv.FormatInt(payment.ID, 10) + ";source=connector_channel_url",
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveAuditEvent access ready: %v", err)
	}
	if err := st.SaveAuditEvent(ctx, domain.AuditEvent{
		TargetUserID: user.ID,
		ConnectorID:  connectorRow.ID,
		Action:       domain.AuditActionSubscriptionRevokeManualCheck,
		Details:      "subscription_id=" + strconv.FormatInt(sub.ID, 10) + ";failed_attempts=3",
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveAuditEvent manual check: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/billing?lang=ru&user_id=1", nil)
	rec := httptest.NewRecorder()
	h.billingPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Доступ / revoke") {
		t.Fatalf("response does not contain access/revoke column: %q", body)
	}
	if !strings.Contains(body, "доступ выдан") {
		t.Fatalf("response does not contain payment access badge: %q", body)
	}
	if !strings.Contains(body, "нужна ручная проверка") {
		t.Fatalf("response does not contain revoke manual-check badge: %q", body)
	}
}
