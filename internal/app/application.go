package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

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

func newApplication(cfg config.Config, st store.Store, opts appInitOptions) (*application, error) {
	tgClient, err := telegram.NewClient(cfg.Telegram.BotToken, cfg.Telegram.Webhook.SecretToken)
	if err != nil {
		return nil, fmt.Errorf("create telegram client: %w", err)
	}
	if opts.checkTransportHealth {
		info, err := tgClient.Ping(context.Background())
		if err != nil {
			return nil, fmt.Errorf("ping telegram api: %w", err)
		}
		if strings.TrimSpace(cfg.Telegram.BotToken) != "" {
			slog.Info("telegram api ping ok", "bot_id", info.ID, "bot_username", info.Username, "bot_name", info.FirstName)
		}
	}
	if opts.ensureTelegramSetup {
		if err := tgClient.EnsureWebhook(context.Background(), cfg.Telegram.Webhook.PublicURL, cfg.Telegram.Webhook.SecretToken); err != nil {
			return nil, fmt.Errorf("ensure telegram webhook: %w", err)
		}
		if err := tgClient.EnsureDefaultMenu(context.Background()); err != nil {
			return nil, fmt.Errorf("ensure telegram menu button: %w", err)
		}
	}
	maxClient := max.NewClient(cfg.MAX.BotToken, nil)
	maxLaunchUsername := strings.TrimSpace(cfg.MAX.BotUsername)
	if opts.checkTransportHealth && strings.TrimSpace(cfg.MAX.BotToken) != "" {
		info, err := maxClient.Ping(context.Background())
		if err != nil {
			return nil, fmt.Errorf("ping MAX api: %w", err)
		}
		slog.Info("MAX api ping ok", "bot_id", info.UserID, "bot_username", info.Username, "bot_name", firstNonEmpty(info.FirstName, info.Name))
		if maxLaunchUsername == "" {
			maxLaunchUsername = strings.TrimSpace(info.Username)
		}
	}
	if opts.ensureMAXSetup {
		if err := maxClient.EnsureWebhook(context.Background(), cfg.MAX.Webhook.PublicURL, cfg.MAX.Webhook.SecretToken, cfg.MAX.Polling.Types); err != nil {
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

	var paymentService payment.Service
	var robokassaService *payment.RobokassaService
	switch cfg.Payment.Provider {
	case "", "mock":
		paymentService = payment.NewMockService(mockBaseURL)
	case "robokassa":
		robokassaService = payment.NewRobokassaService(payment.RobokassaConfig{
			MerchantLogin: cfg.Payment.Robokassa.MerchantLogin,
			Password1:     cfg.Payment.Robokassa.Password1,
			Password2:     cfg.Payment.Robokassa.Password2,
			IsTest:        cfg.Payment.Robokassa.IsTestMode,
			BaseURL:       cfg.Payment.Robokassa.CheckoutURL,
			RebillURL:     cfg.Payment.Robokassa.RebillURL,
		})
		paymentService = robokassaService
	default:
		slog.Warn("payment provider is not implemented yet, fallback to mock", "provider", cfg.Payment.Provider)
		paymentService = payment.NewMockService(mockBaseURL)
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
