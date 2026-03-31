package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jopoleon/invest-control-bot/internal/config"
)

func TestHandleLanding_RendersRootPage(t *testing.T) {
	t.Parallel()

	a := &application{
		config: config.Config{
			AppName: "invest-control-bot",
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	a.handleLanding(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "invest-control-bot") {
		t.Fatalf("landing body does not contain app title: %q", body)
	}
	if !strings.Contains(body, "/admin") {
		t.Fatalf("landing body does not contain admin link: %q", body)
	}
}

func TestHandleLanding_RejectsNonRootPath(t *testing.T) {
	t.Parallel()

	a := &application{}
	req := httptest.NewRequest(http.MethodGet, "/nope", nil)
	rec := httptest.NewRecorder()
	a.handleLanding(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
