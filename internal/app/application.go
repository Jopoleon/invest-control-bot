package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/admin"
	"github.com/Jopoleon/invest-control-bot/internal/bot"
	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/max"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/payment"
	"github.com/Jopoleon/invest-control-bot/internal/store"
	"github.com/Jopoleon/invest-control-bot/internal/telegram"
)

type appInitOptions struct {
	ensureTelegramSetup  bool
	ensureMAXSetup       bool
	checkTransportHealth bool
}

type application struct {
	config             config.Config
	store              store.Store
	telegramClient     *telegram.Client
	maxClient          *max.Client
	maxSender          messenger.Sender
	telegramBotHandler *bot.Handler
	maxBotHandler      *bot.Handler
	maxAdapter         *max.Adapter
	adminHandler       *admin.Handler
	paymentService     payment.Service
	robokassaService   *payment.RobokassaService
}

type startupRetryPolicy struct {
	attempts int
	timeout  time.Duration
	backoff  time.Duration
}

type telegramStartupClient interface {
	Ping(context.Context) (telegram.BotInfo, error)
	EnsureWebhook(context.Context, string, string) error
	EnsureDefaultMenu(context.Context) error
}

const (
	startupRetryAttempts = 3
	startupRetryTimeout  = 10 * time.Second
	startupRetryBackoff  = 1500 * time.Millisecond
)

func newApplication(cfg config.Config, st store.Store, opts appInitOptions) (*application, error) {
	// Telegram transport is built with operational routing options first and
	// only then passed through startup health/setup policy. This keeps relay /
	// proxy concerns localized to client construction instead of leaking
	// transport workarounds into the rest of the app wiring.
	tgClient, err := telegram.NewClientWithOptions(cfg.Telegram.BotToken, cfg.Telegram.Webhook.SecretToken, telegram.ClientOptions{
		ServerURL:    cfg.Telegram.APIBaseURL,
		HTTPProxyURL: cfg.Telegram.HTTPProxyURL,
	})
	if err != nil {
		return nil, fmt.Errorf("create telegram client: %w", err)
	}
	if err := ensureTelegramStartup(context.Background(), cfg, tgClient, opts); err != nil {
		return nil, err
	}
	maxClient := max.NewClient(cfg.MAX.BotToken, nil)
	maxLaunchUsername := strings.TrimSpace(cfg.MAX.BotUsername)
	if opts.checkTransportHealth && strings.TrimSpace(cfg.MAX.BotToken) != "" {
		var info max.BotInfo
		err := runStartupStepWithRetry(context.Background(), "MAX ping", defaultStartupRetryPolicy(), func(ctx context.Context) error {
			var pingErr error
			info, pingErr = maxClient.Ping(ctx)
			return pingErr
		})
		if err != nil {
			return nil, fmt.Errorf("ping MAX api: %w", err)
		}
		slog.Info("MAX api ping ok", "bot_id", info.UserID, "bot_username", info.Username, "bot_name", firstNonEmpty(info.FirstName, info.Name))
		if maxLaunchUsername == "" {
			maxLaunchUsername = strings.TrimSpace(info.Username)
		}
	}
	if opts.ensureMAXSetup {
		if err := runStartupStepWithRetry(context.Background(), "MAX webhook setup", defaultStartupRetryPolicy(), func(ctx context.Context) error {
			return maxClient.EnsureWebhook(ctx, cfg.MAX.Webhook.PublicURL, cfg.MAX.Webhook.SecretToken, cfg.MAX.Polling.Types)
		}); err != nil {
			return nil, fmt.Errorf("ensure MAX webhook: %w", err)
		}
		if strings.TrimSpace(cfg.MAX.Webhook.PublicURL) != "" {
			slog.Info("MAX webhook ensured", "url", cfg.MAX.Webhook.PublicURL, "types", cfg.MAX.Polling.Types)
		}
	}

	mockBaseURL := cfg.Payment.MockBaseURL
	if mockBaseURL == "" {
		mockBaseURL = preferredWebhookURL(cfg)
	}

	paymentService, robokassaService, err := buildPaymentService(cfg, mockBaseURL)
	if err != nil {
		return nil, err
	}

	maxSender := max.NewSender(maxClient)
	publicBase := publicBaseURL(preferredWebhookURL(cfg))

	appCtx := &application{
		config:             cfg,
		store:              st,
		telegramClient:     tgClient,
		maxClient:          maxClient,
		maxSender:          maxSender,
		telegramBotHandler: bot.NewHandler(st, tgClient, paymentService, cfg.Payment.Provider == "robokassa" && cfg.Payment.Robokassa.RecurringEnabled, publicBase, cfg.Security.EncryptionKey),
		maxBotHandler:      bot.NewHandler(st, maxSender, paymentService, cfg.Payment.Provider == "robokassa" && cfg.Payment.Robokassa.RecurringEnabled, publicBase, cfg.Security.EncryptionKey),
		paymentService:     paymentService,
		robokassaService:   robokassaService,
	}
	appCtx.maxAdapter = max.NewAdapter(appCtx.maxBotHandler)
	appCtx.adminHandler = admin.NewHandler(st, cfg.Security.AdminToken, cfg.Telegram.BotUsername, maxLaunchUsername, publicBase, cfg.Security.EncryptionKey, tgClient, maxSender, func(ctx context.Context, subscriptionID int64) (admin.RebillResult, error) {
		payload, err := appCtx.triggerRebill(ctx, subscriptionID, "admin_ui")
		if err != nil {
			return admin.RebillResult{}, err
		}
		return admin.RebillResult{InvoiceID: payload.InvoiceID, Existing: payload.Existing}, nil
	})
	return appCtx, nil
}

func ensureTelegramStartup(ctx context.Context, cfg config.Config, client telegramStartupClient, opts appInitOptions) error {
	if client == nil {
		return nil
	}

	token := strings.TrimSpace(cfg.Telegram.BotToken)
	if token == "" {
		return nil
	}

	transportHealthy := true
	if opts.checkTransportHealth {
		var info telegram.BotInfo
		err := runStartupStepWithRetry(ctx, "telegram ping", defaultStartupRetryPolicy(), func(stepCtx context.Context) error {
			var pingErr error
			info, pingErr = client.Ping(stepCtx)
			return pingErr
		})
		if err != nil {
			if isRetryableStartupError(err) {
				transportHealthy = false
				// Retryable transport failures (timeouts, TLS handshake stalls, etc.)
				// are treated as network degradation, not configuration bugs. We keep
				// the process alive so admin/public/payment paths stay available even
				// when Telegram is temporarily unreachable from the hosting network.
				slog.Warn("telegram startup degraded, continuing without confirmed transport health",
					"step", "ping",
					"error", err,
				)
			} else {
				return fmt.Errorf("ping telegram api: %w", err)
			}
		} else {
			slog.Info("telegram api ping ok", "bot_id", info.ID, "bot_username", info.Username, "bot_name", info.FirstName)
		}
	}

	if !opts.ensureTelegramSetup {
		return nil
	}
	if !transportHealthy {
		// Webhook/menu setup would just fail for the same network reason and would
		// only turn a degraded Telegram transport into a full process outage.
		slog.Warn("skip telegram webhook/menu setup because transport is degraded at startup")
		return nil
	}

	if err := runOptionalTelegramStartupStep(ctx, "webhook setup", func(stepCtx context.Context) error {
		return client.EnsureWebhook(stepCtx, cfg.Telegram.Webhook.PublicURL, cfg.Telegram.Webhook.SecretToken)
	}); err != nil {
		return err
	}
	if err := runOptionalTelegramStartupStep(ctx, "menu setup", client.EnsureDefaultMenu); err != nil {
		return err
	}
	return nil
}

func runOptionalTelegramStartupStep(ctx context.Context, step string, fn func(context.Context) error) error {
	err := runStartupStepWithRetry(ctx, "telegram "+step, defaultStartupRetryPolicy(), fn)
	if err == nil {
		return nil
	}
	if isRetryableStartupError(err) {
		// Setup failures after a healthy client build are still allowed to degrade
		// the process if they look network-related. This keeps startup semantics
		// consistent: retryable transport problems should not take down MAX/admin /
		// payment flows, while non-retryable API/configuration errors still must
		// fail loudly.
		slog.Warn("telegram startup degraded, continuing after retryable setup failure",
			"step", step,
			"error", err,
		)
		return nil
	}
	switch step {
	case "webhook setup":
		return fmt.Errorf("ensure telegram webhook: %w", err)
	case "menu setup":
		return fmt.Errorf("ensure telegram menu button: %w", err)
	default:
		return fmt.Errorf("ensure telegram %s: %w", step, err)
	}
}

func defaultStartupRetryPolicy() startupRetryPolicy {
	return startupRetryPolicy{
		attempts: startupRetryAttempts,
		timeout:  startupRetryTimeout,
		backoff:  startupRetryBackoff,
	}
}

func runStartupStepWithRetry(parent context.Context, step string, policy startupRetryPolicy, fn func(context.Context) error) error {
	attempts := policy.attempts
	if attempts <= 0 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		attemptCtx := parent
		cancel := func() {}
		if policy.timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(parent, policy.timeout)
		}
		err := fn(attemptCtx)
		cancel()
		if err == nil {
			return nil
		}
		if attempt == attempts || !isRetryableStartupError(err) {
			return err
		}
		slog.Warn("startup step failed, retrying",
			"step", step,
			"attempt", attempt,
			"max_attempts", attempts,
			"backoff", policy.backoff,
			"error", err,
		)
		if policy.backoff > 0 {
			time.Sleep(policy.backoff)
		}
	}
	return nil
}

func isRetryableStartupError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "deadline exceeded") ||
		strings.Contains(message, "client.timeout exceeded") ||
		strings.Contains(message, "i/o timeout") ||
		strings.Contains(message, "tls handshake timeout")
}

func buildPaymentService(cfg config.Config, mockBaseURL string) (payment.Service, *payment.RobokassaService, error) {
	switch cfg.Payment.Provider {
	case "", "mock":
		return payment.NewMockService(mockBaseURL), nil, nil
	case "robokassa":
		service := payment.NewRobokassaService(payment.RobokassaConfig{
			MerchantLogin: cfg.Payment.Robokassa.MerchantLogin,
			Password1:     cfg.Payment.Robokassa.Password1,
			Password2:     cfg.Payment.Robokassa.Password2,
			IsTest:        cfg.Payment.Robokassa.IsTestMode,
			BaseURL:       cfg.Payment.Robokassa.CheckoutURL,
			RebillURL:     cfg.Payment.Robokassa.RebillURL,
		})
		return service, service, nil
	default:
		return nil, nil, fmt.Errorf("unsupported payment provider: %s", cfg.Payment.Provider)
	}
}

func publicBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return strings.TrimRight(raw, "/")
	}
	u.RawQuery = ""
	u.Fragment = ""
	u.Path = strings.TrimSuffix(u.Path, "/telegram/webhook")
	u.Path = strings.TrimSuffix(u.Path, "/max/webhook")
	u.Path = strings.TrimRight(u.Path, "/")
	return strings.TrimRight(u.String(), "/")
}

func preferredWebhookURL(cfg config.Config) string {
	switch {
	case strings.TrimSpace(cfg.MAX.Webhook.PublicURL) != "":
		return cfg.MAX.Webhook.PublicURL
	default:
		return cfg.Telegram.Webhook.PublicURL
	}
}
