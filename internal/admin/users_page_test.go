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

func TestBuildMessengerAccountPresentation_PrefersTelegramAsPrimary(t *testing.T) {
	accounts := []domain.UserMessengerAccount{
		{MessengerKind: domain.MessengerKindMAX, MessengerUserID: "193465776", Username: "fedor"},
		{MessengerKind: domain.MessengerKindTelegram, MessengerUserID: "264704572", Username: "emiloserdov"},
	}

	got := buildMessengerAccountPresentation("ru", accounts)

	if got.PrimaryAccount != "Telegram · 264704572 · @emiloserdov" {
		t.Fatalf("PrimaryAccount = %q", got.PrimaryAccount)
	}
	if got.DisplayName != "emiloserdov" {
		t.Fatalf("DisplayName = %q", got.DisplayName)
	}
	if !got.HasTelegramIdentity {
		t.Fatal("expected telegram identity to be detected")
	}
	if len(got.Accounts) != 2 {
		t.Fatalf("accounts len = %d, want 2", len(got.Accounts))
	}
}

func TestUsersPage_ShowsMessengerNeutralAccountSummary(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/admin/users?lang=ru&user_id="+strconv.FormatInt(user.ID, 10), nil)
	rec := httptest.NewRecorder()
	h.usersPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Основной мессенджер") {
		t.Fatalf("response does not contain messenger-neutral column: %q", body)
	}
	if !strings.Contains(body, "MAX · 193465776 · @fedor") {
		t.Fatalf("response does not contain MAX account summary: %q", body)
	}
}

func TestUsersPage_UsesCurrentPeriodAutopayStatusInsteadOfFutureRenewal(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)
	now := time.Now().UTC()

	user, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "264704580", "egor")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger: %v", err)
	}
	user.UpdatedAt = now
	if err := st.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		ID:             1,
		UserID:         user.ID,
		ConnectorID:    11,
		PaymentID:      101,
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
		ID:             2,
		UserID:         user.ID,
		ConnectorID:    11,
		PaymentID:      102,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(time.Hour),
		EndsAt:         now.Add(2 * time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment future: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/users?lang=ru&user_id="+strconv.FormatInt(user.ID, 10), nil)
	rec := httptest.NewRecorder()
	h.usersPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Периоды доступа") || !strings.Contains(body, "Автоплатеж сейчас") {
		t.Fatalf("response does not contain updated users vocabulary: %q", body)
	}
	if !strings.Contains(body, "<span class=\"badge is-warning\">выключен</span>") {
		t.Fatalf("response does not contain current-period autopay status: %q", body)
	}
}
