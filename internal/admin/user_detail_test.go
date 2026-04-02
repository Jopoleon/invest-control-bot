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
