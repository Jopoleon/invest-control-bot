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
	if strings.Contains(body, "telegram_id,telegram_username") {
		t.Fatalf("csv still contains telegram-centric headers: %q", body)
	}
	if !strings.Contains(body, "MAX · 193465776 · @fedor") {
		t.Fatalf("csv does not contain MAX account summary: %q", body)
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
