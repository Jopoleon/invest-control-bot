package app

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
)

type fakeTelegramStartupClient struct {
	pingInfo telegram.BotInfo
	pingErr  error

	webhookErr error
	menuErr    error

	webhookCalls int
	menuCalls    int
}

func (c *fakeTelegramStartupClient) Ping(context.Context) (telegram.BotInfo, error) {
	return c.pingInfo, c.pingErr
}

func (c *fakeTelegramStartupClient) EnsureWebhook(context.Context, string, string) error {
	c.webhookCalls++
	return c.webhookErr
}

func (c *fakeTelegramStartupClient) EnsureDefaultMenu(context.Context) error {
	c.menuCalls++
	return c.menuErr
}

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "i/o timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

func TestEnsureTelegramStartup_DegradesOnRetryablePingError(t *testing.T) {
	client := &fakeTelegramStartupClient{pingErr: timeoutNetError{}}
	cfg := config.Config{
		Telegram: config.TelegramConfig{
			BotToken: "test-token",
			Webhook: config.WebhookConfig{
				PublicURL:   "https://example.test/telegram/webhook",
				SecretToken: "secret",
			},
		},
	}

	err := ensureTelegramStartup(context.Background(), cfg, client, appInitOptions{
		checkTransportHealth: true,
		ensureTelegramSetup:  true,
	})
	if err != nil {
		t.Fatalf("ensureTelegramStartup: %v", err)
	}
	if client.webhookCalls != 0 {
		t.Fatalf("expected webhook setup to be skipped, got %d calls", client.webhookCalls)
	}
	if client.menuCalls != 0 {
		t.Fatalf("expected menu setup to be skipped, got %d calls", client.menuCalls)
	}
}

func TestEnsureTelegramStartup_FailsOnNonRetryablePingError(t *testing.T) {
	client := &fakeTelegramStartupClient{pingErr: errors.New("unauthorized")}
	cfg := config.Config{
		Telegram: config.TelegramConfig{
			BotToken: "test-token",
		},
	}

	err := ensureTelegramStartup(context.Background(), cfg, client, appInitOptions{
		checkTransportHealth: true,
	})
	if err == nil {
		t.Fatal("expected non-retryable ping error")
	}
}

func TestEnsureTelegramStartup_DegradesOnRetryableWebhookError(t *testing.T) {
	client := &fakeTelegramStartupClient{
		pingInfo:     telegram.BotInfo{ID: 1, Username: "bot"},
		webhookErr:   &net.DNSError{IsTimeout: true},
		menuErr:      nil,
		webhookCalls: 0,
	}
	cfg := config.Config{
		Telegram: config.TelegramConfig{
			BotToken: "test-token",
			Webhook: config.WebhookConfig{
				PublicURL:   "https://example.test/telegram/webhook",
				SecretToken: "secret",
			},
		},
	}

	err := ensureTelegramStartup(context.Background(), cfg, client, appInitOptions{
		checkTransportHealth: true,
		ensureTelegramSetup:  true,
	})
	if err != nil {
		t.Fatalf("ensureTelegramStartup: %v", err)
	}
	if client.webhookCalls != startupRetryAttempts {
		t.Fatalf("expected webhook setup to consume retry budget %d times, got %d calls", startupRetryAttempts, client.webhookCalls)
	}
	if client.menuCalls != 1 {
		t.Fatalf("expected menu setup to continue after degraded webhook, got %d calls", client.menuCalls)
	}
}

func TestEnsureTelegramStartup_FailsOnNonRetryableMenuError(t *testing.T) {
	client := &fakeTelegramStartupClient{
		pingInfo: telegram.BotInfo{ID: 1, Username: "bot"},
		menuErr:  errors.New("bad request"),
	}
	cfg := config.Config{
		Telegram: config.TelegramConfig{
			BotToken: "test-token",
			Webhook: config.WebhookConfig{
				PublicURL: "https://example.test/telegram/webhook",
			},
		},
	}

	err := ensureTelegramStartup(context.Background(), cfg, client, appInitOptions{
		checkTransportHealth: true,
		ensureTelegramSetup:  true,
	})
	if err == nil {
		t.Fatal("expected non-retryable menu setup error")
	}
}
