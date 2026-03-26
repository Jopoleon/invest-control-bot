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

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
)

func TestSendUserMessage_AllowsUserIDResolution(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	tg, err := telegram.NewClient("", "")
	if err != nil {
		t.Fatalf("telegram.NewClient: %v", err)
	}
	h := NewHandler(st, "test-admin-token", "test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", tg, nil)

	if err := st.SaveUser(ctx, domain.User{
		TelegramID:       264704572,
		TelegramUsername: "emiloserdov",
		FullName:         "Egor Miloserdov",
		UpdatedAt:        time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	user, found, err := st.GetUser(ctx, 264704572)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if !found {
		t.Fatal("expected saved user")
	}

	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/users/view?lang=ru&user_id="+strconv.FormatInt(user.ID, 10), nil)
	csrfRec := httptest.NewRecorder()
	csrfToken := h.ensureCSRFToken(csrfRec, csrfReq)
	csrfResp := csrfRec.Result()
	defer csrfResp.Body.Close()

	var csrfCookie *http.Cookie
	for _, c := range csrfResp.Cookies() {
		if c.Name == csrfCookieName {
			csrfCookie = c
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("csrf cookie missing")
	}

	form := url.Values{}
	form.Set("csrf_token", csrfToken)
	form.Set("user_id", strconv.FormatInt(user.ID, 10))
	form.Set("message", "test message from admin")

	req := httptest.NewRequest(http.MethodPost, "/admin/users/message?lang=ru", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	h.sendUserMessage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "сообщение отправлено пользователю") {
		t.Fatalf("response does not contain success notice: %q", rec.Body.String())
	}
}
