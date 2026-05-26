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

func TestExportUsersCSV_UsesMessengerNeutralColumns(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)
	now := time.Now().UTC()

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindMAX, "193465776", "fedor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger MAX: %v", err)
	}
	if _, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "264704572", "emiloserdov"); err != nil {
		t.Fatalf("GetOrCreateUserByMessenger Telegram: %v", err)
	}
	user.FullName = "Fedor"
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		ID:          101,
		UserID:      user.ID,
		ConnectorID: 1,
		PaymentID:   501,
		Status:      domain.SubscriptionStatusActive,
		StartsAt:    now.Add(-time.Hour),
		EndsAt:      now.Add(time.Hour),
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment current: %v", err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		ID:          102,
		UserID:      user.ID,
		ConnectorID: 1,
		PaymentID:   502,
		Status:      domain.SubscriptionStatusActive,
		StartsAt:    now.Add(time.Hour),
		EndsAt:      now.Add(2 * time.Hour),
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment next: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/users/export.csv?lang=ru", nil)
	rec := httptest.NewRecorder()
	h.exportUsersCSV(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "primary_account") {
		t.Fatalf("csv header does not contain primary_account: %q", body)
	}
	if !strings.Contains(body, "current_periods,next_periods,auto_pay_enabled") {
		t.Fatalf("csv header does not contain phase-aware user columns: %q", body)
	}
	if strings.Contains(body, "telegram_id,telegram_username") {
		t.Fatalf("csv still contains telegram-centric headers: %q", body)
	}
	if !strings.Contains(body, "MAX · 193465776 · @fedor") {
		t.Fatalf("csv does not contain MAX account summary: %q", body)
	}
	if !strings.Contains(body, ",1,1,") {
		t.Fatalf("csv does not contain current/next period counts: %q", body)
	}
}

func TestExportUsersCSV_UsesCurrentPeriodAutopayStateInsteadOfFutureRenewal(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)
	now := time.Now().UTC()

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "264704590", "phase")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		ID:             111,
		UserID:         user.ID,
		ConnectorID:    1,
		PaymentID:      611,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: false,
		StartsAt:       now.Add(-time.Hour),
		EndsAt:         now.Add(time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment current: %v", err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		ID:             112,
		UserID:         user.ID,
		ConnectorID:    1,
		PaymentID:      612,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(time.Hour),
		EndsAt:         now.Add(2 * time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment future: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/users/export.csv?lang=ru", nil)
	rec := httptest.NewRecorder()
	h.exportUsersCSV(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, ",1,1,false,true,") {
		t.Fatalf("csv does not contain phase-aware autopay state: %q", body)
	}
}

func TestExportSubscriptionsCSV_IncludesPhaseAndCurrentFirst(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)
	now := time.Now().UTC()

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "264704572", "egor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-export-subscriptions",
		Name:          "Export Connector",
		ChatID:        "1001234567890",
		PriceRUB:      100,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 3600,
		IsActive:      true,
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-export-subscriptions")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}
	if err := st.UpsertTelegramChat(ctx, domain.TelegramChat{
		ChatID: "-1001234567890",
		Title:  "Export Project Chat",
		Type:   "supergroup",
	}); err != nil {
		t.Fatalf("UpsertTelegramChat: %v", err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		ID:          201,
		UserID:      user.ID,
		ConnectorID: connector.ID,
		PaymentID:   601,
		Status:      domain.SubscriptionStatusActive,
		StartsAt:    now.Add(time.Hour),
		EndsAt:      now.Add(2 * time.Hour),
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment future: %v", err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		ID:          202,
		UserID:      user.ID,
		ConnectorID: connector.ID,
		PaymentID:   602,
		Status:      domain.SubscriptionStatusActive,
		StartsAt:    now.Add(-time.Hour),
		EndsAt:      now.Add(time.Hour),
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment current: %v", err)
	}
	for _, payment := range []domain.Payment{
		{Provider: "robokassa", Status: domain.PaymentStatusPaid, Token: "export-sub-payment-1", UserID: user.ID, ConnectorID: connector.ID, AmountRUB: 100, CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-2 * time.Hour)},
		{Provider: "robokassa", Status: domain.PaymentStatusPaid, Token: "export-sub-payment-2", UserID: user.ID, ConnectorID: connector.ID, AmountRUB: 100, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)},
	} {
		if err := st.CreatePayment(ctx, payment); err != nil {
			t.Fatalf("CreatePayment: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/billing/subscriptions/export.csv?lang=ru&user_id=1", nil)
	rec := httptest.NewRecorder()
	h.exportSubscriptionsCSV(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "status,phase,auto_pay_enabled") {
		t.Fatalf("csv header does not contain phase column: %q", body)
	}
	if !strings.Contains(body, "connector,project,messenger_kind") {
		t.Fatalf("csv header does not contain project column near connector/messenger fields: %q", body)
	}
	if !strings.Contains(body, "messenger_kind,messenger_user_id,messenger_username,telegram_username") {
		t.Fatalf("csv header does not contain messenger identity columns: %q", body)
	}
	if strings.Contains(body, "starts_at,ends_at") {
		t.Fatalf("csv still exports access-period columns instead of next_payment_at: %q", body)
	}
	if !strings.Contains(body, "monthly_payment_rub,total_paid_payments,next_payment_at") {
		t.Fatalf("csv header does not contain payment summary columns: %q", body)
	}
	if !strings.Contains(body, "Export Connector,Export Project Chat") {
		t.Fatalf("csv does not contain project resolved from telegram chat title: %q", body)
	}
	if !strings.Contains(body, "telegram,264704572,egor,egor") {
		t.Fatalf("csv does not contain messenger usernames: %q", body)
	}
	if !strings.Contains(body, ",100,2,") {
		t.Fatalf("csv does not contain monthly amount and paid payment count: %q", body)
	}
	if !strings.Contains(body, ",current,") || !strings.Contains(body, ",next,") {
		t.Fatalf("csv does not contain current/next phase values: %q", body)
	}
	currentPos := strings.Index(body, ",602,active,current,")
	futurePos := strings.Index(body, ",601,active,next,")
	if currentPos == -1 || futurePos == -1 {
		t.Fatalf("csv does not contain expected payment/phase rows: %q", body)
	}
	if currentPos > futurePos {
		t.Fatalf("current subscription exported after next renewal: current=%d future=%d body=%q", currentPos, futurePos, body)
	}
}

func TestExportPaymentsCSV_IncludesProjectFromTelegramChatTitle(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)
	now := time.Now().UTC()

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "464152205", "mishka")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload: "in-export-payments-project",
		Name:         "Retail Connector",
		ChatID:       "1009876543210",
		PriceRUB:     3000,
		PeriodMode:   domain.ConnectorPeriodModeCalendarMonths,
		PeriodMonths: 1,
		IsActive:     true,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-export-payments-project")
	if err != nil || !found {
		t.Fatalf("GetConnectorByStartPayload found=%v err=%v", found, err)
	}
	if err := st.UpsertTelegramChat(ctx, domain.TelegramChat{
		ChatID: "-1009876543210",
		Title:  "Retail Project Chat",
		Type:   "supergroup",
	}); err != nil {
		t.Fatalf("UpsertTelegramChat: %v", err)
	}
	if err := st.CreatePayment(ctx, domain.Payment{
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "export-payment-project",
		UserID:         user.ID,
		ConnectorID:    connector.ID,
		AmountRUB:      3000,
		AutoPayEnabled: true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/billing/payments/export.csv?lang=ru&user_id=1", nil)
	rec := httptest.NewRecorder()
	h.exportPaymentsCSV(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "connector,project,messenger_kind") {
		t.Fatalf("csv header does not contain project column near connector/messenger fields: %q", body)
	}
	if !strings.Contains(body, "messenger_kind,messenger_user_id,messenger_username,telegram_username") {
		t.Fatalf("csv header does not contain messenger identity columns: %q", body)
	}
	if !strings.Contains(body, "monthly_payment_rub,total_paid_payments") {
		t.Fatalf("csv header does not contain payment summary columns: %q", body)
	}
	if !strings.Contains(body, "Retail Connector,Retail Project Chat") {
		t.Fatalf("csv does not contain project resolved from telegram chat title: %q", body)
	}
	if !strings.Contains(body, "telegram,464152205,mishka,mishka") {
		t.Fatalf("csv does not contain messenger usernames: %q", body)
	}
	if !strings.Contains(body, ",3000,1,") {
		t.Fatalf("csv does not contain monthly amount and paid payment count: %q", body)
	}
}

func TestExportEventsCSV_IncludesMessengerKindAndAccount(t *testing.T) {
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
		t.Fatalf("SaveAuditEvent: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/events/export.csv?lang=ru&messenger_kind=max", nil)
	rec := httptest.NewRecorder()
	h.exportEventsCSV(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "actor_user_id,actor_messenger_kind,actor_messenger_user_id,actor_subject,target_user_id,target_messenger_kind,target_messenger_user_id,target_account") {
		t.Fatalf("csv header does not contain full actor/target audit columns: %q", body)
	}
	if !strings.Contains(body, "max,193465776,MAX · 193465776") {
		t.Fatalf("csv does not contain MAX target account summary: %q", body)
	}
}

func TestParseEventsQuery_UsesExplicitMessengerKind(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/events?messenger_kind=max&messenger_user_id=193465776", nil)
	query := parseEventsQuery(req.URL.Query())

	if query.TargetMessengerKind != domain.MessengerKindMAX {
		t.Fatalf("TargetMessengerKind = %q, want %q", query.TargetMessengerKind, domain.MessengerKindMAX)
	}
	if query.TargetMessengerUserID != "193465776" {
		t.Fatalf("TargetMessengerUserID = %q", query.TargetMessengerUserID)
	}
}
