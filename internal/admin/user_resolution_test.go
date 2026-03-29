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

func TestResolveFilterTelegramID_UsesUserID(t *testing.T) {
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

	telegramID, err := h.resolveFilterTelegramID(ctx, strconv.FormatInt(user.ID, 10), "")
	if err != nil {
		t.Fatalf("resolveFilterTelegramID: %v", err)
	}
	if telegramID != 264704572 {
		t.Fatalf("telegramID = %d, want %d", telegramID, 264704572)
	}
}

func TestUsersPage_FiltersByUserID(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil)

	first, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "111", "")
	if err != nil {
		t.Fatalf("GetOrCreate first: %v", err)
	}
	first.FullName = "First User"
	first.UpdatedAt = time.Now().UTC()
	if err := st.SaveUser(ctx, first); err != nil {
		t.Fatalf("SaveUser first: %v", err)
	}
	second, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "222", "")
	if err != nil {
		t.Fatalf("GetOrCreate second: %v", err)
	}
	second.FullName = "Second User"
	second.UpdatedAt = time.Now().UTC()
	if err := st.SaveUser(ctx, second); err != nil {
		t.Fatalf("SaveUser second: %v", err)
	}
	target, found, err := st.GetUser(ctx, 222)
	if err != nil {
		t.Fatalf("GetUser target: %v", err)
	}
	if !found {
		t.Fatal("expected target user")
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/users?lang=ru&user_id="+strconv.FormatInt(target.ID, 10), nil)
	rec := httptest.NewRecorder()
	h.usersPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Second User") {
		t.Fatalf("response does not contain filtered user: %q", body)
	}
	if strings.Contains(body, "First User") {
		t.Fatalf("response still contains unrelated user: %q", body)
	}
}
