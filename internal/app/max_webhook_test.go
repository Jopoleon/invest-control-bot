package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jopoleon/invest-control-bot/internal/bot"
	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/max"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
)

func TestHandleMAXWebhookDispatchesIntoBotCore(t *testing.T) {
	st := memory.New()
	sender := &spySender{}
	appCtx := &application{
		config: config.Config{
			MAX: config.MAXConfig{
				Webhook: config.WebhookConfig{SecretToken: "max-secret"},
			},
		},
		maxBotHandler: bot.NewHandler(st, sender, nil, false, "http://localhost:8080", "test-encryption-key-123456789012345"),
	}
	appCtx.maxAdapter = max.NewAdapter(appCtx.maxBotHandler)

	req := httptest.NewRequest(http.MethodPost, "/max/webhook", strings.NewReader(`{
		"update_type":"message_created",
		"message":{
			"sender":{"user_id":193465776,"username":"egor"},
			"recipient":{"chat_id":109778209,"chat_type":"dialog","user_id":218306705},
			"body":{"mid":"123","text":"/menu"}
		}
	}`))
	req.Header.Set("X-Max-Bot-Api-Secret", "max-secret")
	rr := httptest.NewRecorder()

	appCtx.handleMAXWebhook(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(sender.sent))
	}
	if got := sender.sent[0].msg.Text; !strings.Contains(got, "Личный кабинет") {
		t.Fatalf("sent text = %q", got)
	}
}

func TestHandleMAXWebhookRejectsInvalidSecret(t *testing.T) {
	appCtx := &application{
		config: config.Config{
			MAX: config.MAXConfig{
				Webhook: config.WebhookConfig{SecretToken: "max-secret"},
			},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/max/webhook", strings.NewReader(`{}`))
	req.Header.Set("X-Max-Bot-Api-Secret", "wrong-secret")
	rr := httptest.NewRecorder()

	appCtx.handleMAXWebhook(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}
