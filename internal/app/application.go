package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Jopoleon/telega-bot-fedor/internal/admin"
	"github.com/Jopoleon/telega-bot-fedor/internal/bot"
	"github.com/Jopoleon/telega-bot-fedor/internal/config"
	"github.com/Jopoleon/telega-bot-fedor/internal/payment"
	"github.com/Jopoleon/telega-bot-fedor/internal/store"
	"github.com/Jopoleon/telega-bot-fedor/internal/telegram"
)

type application struct {
	config           config.Config
	store            store.Store
	telegramClient   *telegram.Client
	botHandler       *bot.Handler
	adminHandler     *admin.Handler
	paymentService   payment.Service
	robokassaService *payment.RobokassaService
}

func newApplication(cfg config.Config, st store.Store) (*application, error) {
	tgClient, err := telegram.NewClient(cfg.Telegram.BotToken, cfg.Telegram.Webhook.SecretToken)
	if err != nil {
		return nil, fmt.Errorf("create telegram client: %w", err)
	}
	if err := tgClient.EnsureWebhook(context.Background(), cfg.Telegram.Webhook.PublicURL, cfg.Telegram.Webhook.SecretToken); err != nil {
		return nil, fmt.Errorf("ensure telegram webhook: %w", err)
	}
	if err := tgClient.EnsureDefaultMenu(context.Background()); err != nil {
		return nil, fmt.Errorf("ensure telegram menu button: %w", err)
	}

	mockBaseURL := cfg.Payment.MockBaseURL
	if mockBaseURL == "" {
		mockBaseURL = cfg.Telegram.Webhook.PublicURL
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

	return &application{
		config:           cfg,
		store:            st,
		telegramClient:   tgClient,
		botHandler:       bot.NewHandler(st, tgClient, paymentService, cfg.Payment.Provider == "robokassa" && cfg.Payment.Robokassa.RecurringEnabled),
		adminHandler:     admin.NewHandler(st, cfg.Security.AdminToken, cfg.Telegram.BotUsername, tgClient),
		paymentService:   paymentService,
		robokassaService: robokassaService,
	}, nil
}
