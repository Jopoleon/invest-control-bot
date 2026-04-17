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

func TestConnectorsPageCreate_UsesMonthlyPresetFields(t *testing.T) {
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
	form.Set("name", "Monthly Plan")
	form.Set("price_rub", "490")
	form.Set("period_mode", "calendar_months")
	form.Set("period_months_preset", "3")
	form.Set("channel_url", "https://t.me/testmonthly")

	req := httptest.NewRequest(http.MethodPost, "/admin/connectors?lang=ru", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	h.connectorsPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}

	connectors, err := st.ListConnectors(req.Context())
	if err != nil {
		t.Fatalf("ListConnectors() error = %v", err)
	}
	if got := len(connectors); got != 1 {
		t.Fatalf("connector count = %d, want 1", got)
	}
	if connectors[0].PeriodMode != domain.ConnectorPeriodModeCalendarMonths || connectors[0].PeriodMonths != 3 {
		t.Fatalf("period = (%q,%d), want (calendar_months,3)", connectors[0].PeriodMode, connectors[0].PeriodMonths)
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

func TestConnectorsPage_DefaultSortShowsActiveNewestFirst(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	connectors := []domain.Connector{
		{
			ID:            1,
			StartPayload:  "in-old-active",
			Name:          "Old Active",
			ChannelURL:    "https://t.me/oldactive",
			PriceRUB:      100,
			PeriodMode:    domain.ConnectorPeriodModeDuration,
			PeriodSeconds: 600,
			IsActive:      true,
			CreatedAt:     time.Now().UTC(),
		},
		{
			ID:            2,
			StartPayload:  "in-inactive",
			Name:          "Inactive",
			ChannelURL:    "https://t.me/inactive",
			PriceRUB:      100,
			PeriodMode:    domain.ConnectorPeriodModeDuration,
			PeriodSeconds: 600,
			IsActive:      false,
			CreatedAt:     time.Now().UTC(),
		},
		{
			ID:            3,
			StartPayload:  "in-new-active",
			Name:          "New Active",
			ChannelURL:    "https://t.me/newactive",
			PriceRUB:      100,
			PeriodMode:    domain.ConnectorPeriodModeDuration,
			PeriodSeconds: 600,
			IsActive:      true,
			CreatedAt:     time.Now().UTC(),
		},
	}
	for _, connector := range connectors {
		if err := st.CreateConnector(ctx, connector); err != nil {
			t.Fatalf("CreateConnector: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/connectors?lang=ru", nil)
	rec := httptest.NewRecorder()
	h.connectorsPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	newPos := strings.Index(body, "New Active")
	oldPos := strings.Index(body, "Old Active")
	inactivePos := strings.Index(body, "Inactive")
	if newPos == -1 || oldPos == -1 || inactivePos == -1 {
		t.Fatalf("connector names missing in body: %q", body)
	}
	if !(newPos < oldPos && oldPos < inactivePos) {
		t.Fatalf("unexpected connector order: new=%d old=%d inactive=%d", newPos, oldPos, inactivePos)
	}
}

func TestConnectorsPage_FiltersDualDestinationConnectors(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	for _, connector := range []domain.Connector{
		{
			StartPayload:  "in-telegram-only",
			Name:          "Telegram Only",
			ChannelURL:    "https://t.me/testtelegram",
			PriceRUB:      100,
			PeriodMode:    domain.ConnectorPeriodModeDuration,
			PeriodSeconds: 600,
			IsActive:      true,
			CreatedAt:     time.Now().UTC(),
		},
		{
			StartPayload:  "in-max-only",
			Name:          "MAX Only",
			MAXChannelURL: "https://max.ru/-72598909498032",
			PriceRUB:      100,
			PeriodMode:    domain.ConnectorPeriodModeDuration,
			PeriodSeconds: 600,
			IsActive:      true,
			CreatedAt:     time.Now().UTC(),
		},
		{
			StartPayload:  "in-dual",
			Name:          "Dual Connector",
			ChannelURL:    "https://t.me/testdual",
			MAXChannelURL: "https://max.ru/-72598909498033",
			PriceRUB:      100,
			PeriodMode:    domain.ConnectorPeriodModeDuration,
			PeriodSeconds: 600,
			IsActive:      true,
			CreatedAt:     time.Now().UTC(),
		},
	} {
		if err := st.CreateConnector(ctx, connector); err != nil {
			t.Fatalf("CreateConnector: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/connectors?lang=ru&destination=dual", nil)
	rec := httptest.NewRecorder()
	h.connectorsPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Dual Connector") {
		t.Fatalf("dual connector missing from filtered response: %q", body)
	}
	if strings.Contains(body, "Telegram Only") || strings.Contains(body, "MAX Only") {
		t.Fatalf("filtered response contains non-dual connectors: %q", body)
	}
	if !strings.Contains(body, "value=\"dual\" selected") {
		t.Fatalf("response does not keep destination filter selection: %q", body)
	}
}

func TestConnectorsPage_RendersOperationalUsageForConnector(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	h := NewHandler(st, "test-admin-token", "test_bot", "max_test_bot", "http://localhost:8080", "test-encryption-key-123456789012345", nil, nil, nil)

	if err := st.CreateConnector(ctx, domain.Connector{
		StartPayload:  "in-usage",
		Name:          "Usage Connector",
		ChannelURL:    "https://t.me/usageconnector",
		PriceRUB:      100,
		PeriodMode:    domain.ConnectorPeriodModeDuration,
		PeriodSeconds: 3600,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateConnector: %v", err)
	}
	connector, found, err := st.GetConnectorByStartPayload(ctx, "in-usage")
	if err != nil {
		t.Fatalf("GetConnectorByStartPayload: %v", err)
	}
	if !found {
		t.Fatal("connector not found after create")
	}

	currentUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "111", "alice")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger current: %v", err)
	}
	nextUser, _, err := st.GetOrCreateUserByMessenger(ctx, domain.MessengerKindTelegram, "222", "bob")
	if err != nil {
		t.Fatalf("GetOrCreateUserByMessenger next: %v", err)
	}

	now := time.Now().UTC()
	if err := st.CreatePayment(ctx, domain.Payment{
		ID:             101,
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "paid-current",
		UserID:         currentUser.ID,
		ConnectorID:    connector.ID,
		AmountRUB:      100,
		CreatedAt:      now,
		UpdatedAt:      now,
		PaidAt:         &now,
		AutoPayEnabled: true,
	}); err != nil {
		t.Fatalf("CreatePayment current: %v", err)
	}
	if err := st.CreatePayment(ctx, domain.Payment{
		ID:             102,
		Provider:       "robokassa",
		Status:         domain.PaymentStatusPaid,
		Token:          "paid-next",
		UserID:         nextUser.ID,
		ConnectorID:    connector.ID,
		AmountRUB:      100,
		CreatedAt:      now,
		UpdatedAt:      now,
		PaidAt:         &now,
		AutoPayEnabled: true,
	}); err != nil {
		t.Fatalf("CreatePayment next: %v", err)
	}

	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		ID:             201,
		UserID:         currentUser.ID,
		ConnectorID:    connector.ID,
		PaymentID:      101,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(-30 * time.Minute),
		EndsAt:         now.Add(30 * time.Minute),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment current: %v", err)
	}
	if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		ID:             202,
		UserID:         nextUser.ID,
		ConnectorID:    connector.ID,
		PaymentID:      102,
		Status:         domain.SubscriptionStatusActive,
		AutoPayEnabled: true,
		StartsAt:       now.Add(30 * time.Minute),
		EndsAt:         now.Add(90 * time.Minute),
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertSubscriptionByPayment next: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/connectors?lang=ru", nil)
	rec := httptest.NewRecorder()
	h.connectorsPage(rec, withAdminAuthorized(req, &authorizedSession{session: domain.AdminSession{ID: 1}}))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, needle := range []string{
		"Использование",
		"Пользователи",
		"Оплачено",
		"Текущие периоды",
		"Следующие периоды",
		"Автоплатеж сейчас: 1",
		"Автоплатеж следующий: 1",
		"@alice",
		"@bob",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("response does not contain %q: %q", needle, body)
		}
	}
}

func TestBuildConnectorUsageViews_SplitsAutopayBetweenCurrentAndNextPeriods(t *testing.T) {
	now := time.Now().UTC()
	usage := buildConnectorUsageViews(
		context.Background(),
		[]domain.Connector{{ID: 77, Name: "Test"}},
		[]domain.Payment{
			{ID: 1, UserID: 10, ConnectorID: 77, Status: domain.PaymentStatusPaid},
			{ID: 2, UserID: 20, ConnectorID: 77, Status: domain.PaymentStatusPaid},
		},
		[]domain.Subscription{
			{
				ID:             11,
				UserID:         10,
				ConnectorID:    77,
				Status:         domain.SubscriptionStatusActive,
				AutoPayEnabled: true,
				StartsAt:       now.Add(-time.Hour),
				EndsAt:         now.Add(time.Hour),
			},
			{
				ID:             12,
				UserID:         20,
				ConnectorID:    77,
				Status:         domain.SubscriptionStatusActive,
				AutoPayEnabled: true,
				StartsAt:       now.Add(time.Hour),
				EndsAt:         now.Add(2 * time.Hour),
			},
			{
				ID:             13,
				UserID:         30,
				ConnectorID:    77,
				Status:         domain.SubscriptionStatusExpired,
				AutoPayEnabled: true,
				StartsAt:       now.Add(-3 * time.Hour),
				EndsAt:         now.Add(-2 * time.Hour),
			},
		},
		now,
		func(userID int64) (messengerAccountPresentation, error) {
			return messengerAccountPresentation{PrimaryAccount: "user #" + strconv.FormatInt(userID, 10)}, nil
		},
	)

	item := usage[77]
	if item.DistinctUsers != 3 {
		t.Fatalf("DistinctUsers = %d, want 3", item.DistinctUsers)
	}
	if item.PaidPayments != 2 {
		t.Fatalf("PaidPayments = %d, want 2", item.PaidPayments)
	}
	if item.CurrentPeriods != 1 {
		t.Fatalf("CurrentPeriods = %d, want 1", item.CurrentPeriods)
	}
	if item.NextPeriods != 1 {
		t.Fatalf("NextPeriods = %d, want 1", item.NextPeriods)
	}
	if item.CurrentAutoPay != 1 {
		t.Fatalf("CurrentAutoPay = %d, want 1", item.CurrentAutoPay)
	}
	if item.NextAutoPay != 1 {
		t.Fatalf("NextAutoPay = %d, want 1", item.NextAutoPay)
	}
	if got := len(item.CurrentUsers); got != 1 {
		t.Fatalf("CurrentUsers len = %d, want 1", got)
	}
	if got := len(item.NextUsers); got != 1 {
		t.Fatalf("NextUsers len = %d, want 1", got)
	}
}
