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

func TestBuildRecurringSummary_EnabledWithoutConsent(t *testing.T) {
	got := buildRecurringSummary("ru", true, true, nil, map[int64]string{}, nil, nil)
	if got.HealthClass != "is-danger" {
		t.Fatalf("HealthClass = %q, want %q", got.HealthClass, "is-danger")
	}
}

func TestBuildRecurringSummary_UsesLatestConsent(t *testing.T) {
	now := time.Now().UTC()
	got := buildRecurringSummary("ru", true, true, []domain.RecurringConsent{
		{ConnectorID: 2, AcceptedAt: now},
	}, map[int64]string{2: "connector-2"}, nil, nil)
	if got.LastConsentConnector != "connector-2" {
		t.Fatalf("LastConsentConnector = %q, want %q", got.LastConsentConnector, "connector-2")
	}
	if got.HealthClass != "is-success" {
		t.Fatalf("HealthClass = %q, want %q", got.HealthClass, "is-success")
	}
}

func TestBuildRecurringSummary_UsesRebillState(t *testing.T) {
	now := time.Now().UTC()
	got := buildRecurringSummary("ru", true, true, nil, map[int64]string{}, []domain.Payment{
		{
			SubscriptionID:  11,
			ParentPaymentID: 1,
			Status:          domain.PaymentStatusFailed,
			CreatedAt:       now,
		},
	}, []domain.Subscription{
		{ID: 11, AutoPayEnabled: true},
	})
	if got.LastRebillLabel != "последний rebill с ошибкой" {
		t.Fatalf("LastRebillLabel = %q", got.LastRebillLabel)
	}
	if got.FailedAttempts != 1 {
		t.Fatalf("FailedAttempts = %d", got.FailedAttempts)
	}
}

func TestBuildUserDetailURL_IncludesUserIDAndTelegramID(t *testing.T) {
	got := buildUserDetailURL("ru", 17, 264704572)
	if !strings.Contains(got, "user_id=17") {
		t.Fatalf("user_id missing from url: %q", got)
	}
	if !strings.Contains(got, "telegram_id=264704572") {
		t.Fatalf("telegram_id missing from url: %q", got)
	}
}

func TestUserDetailPage_AllowsUserIDLookup(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil)

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

	req := httptest.NewRequest(http.MethodGet, "/admin/users/view?lang=ru&user_id="+strconv.FormatInt(user.ID, 10), nil)
	rec := httptest.NewRecorder()
	h.userDetailPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), user.FullName) {
		t.Fatalf("response does not contain user full name: %q", rec.Body.String())
	}
}
