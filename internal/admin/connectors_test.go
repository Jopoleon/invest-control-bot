package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestConnectorsPageCreate_UsesRedirectAfterPost(t *testing.T) {
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/connectors?lang=ru", nil)
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
	form.Set("name", "MAX Test")
	form.Set("price_rub", "3200")
	form.Set("period_mode", "duration")
	form.Set("period_value", "15m")
	form.Set("channel_url", "https://web.max.ru/-72598909498032")

	req := httptest.NewRequest(http.MethodPost, "/admin/connectors?lang=ru", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	h.connectorsPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get("Location")
	if !strings.HasPrefix(location, "/admin/connectors?lang=ru&notice=") {
		t.Fatalf("redirect location = %q", location)
	}

	connectors, err := st.ListConnectors(req.Context())
	if err != nil {
		t.Fatalf("ListConnectors() error = %v", err)
	}
	if got := len(connectors); got != 1 {
		t.Fatalf("connector count = %d, want 1", got)
	}
	if connectors[0].StartPayload == "" || !strings.HasPrefix(connectors[0].StartPayload, "in-") {
		t.Fatalf("generated start payload = %q, want in-*", connectors[0].StartPayload)
	}
	if connectors[0].PeriodMode != domain.ConnectorPeriodModeDuration || connectors[0].PeriodSeconds != 900 {
		t.Fatalf("explicit period = (%q,%d), want (duration,900)", connectors[0].PeriodMode, connectors[0].PeriodSeconds)
	}
}

func TestConnectorsPage_RendersCompactLaunchAndLegalColumns(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-compact-table",
		Name:          "Compact Table",
		ChatID:        "-100123",
		ChannelURL:    "https://t.me/testtestinvest",
		PriceRUB:      3200,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 30 * 24 * 60 * 60,
		OfferURL:      "https://example.com/offer",
		PrivacyURL:    "https://example.com/privacy",
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/connectors?lang=ru", nil)
	rec := httptest.NewRecorder()
	h.connectorsPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, ">Документы<") {
		t.Fatalf("response does not contain compact legal column: %q", body)
	}
	if !strings.Contains(body, ">Запуск<") {
		t.Fatalf("response does not contain compact launch column: %q", body)
	}
	if strings.Contains(body, "<th>Оферта</th>") || strings.Contains(body, "<th>Политика</th>") {
		t.Fatalf("response still contains separate legal columns: %q", body)
	}
}
