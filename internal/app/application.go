package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

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

	appCtx := &application{
		config:           cfg,
		store:            st,
		telegramClient:   tgClient,
		botHandler:       bot.NewHandler(st, tgClient, paymentService, cfg.Payment.Provider == "robokassa" && cfg.Payment.Robokassa.RecurringEnabled, publicBaseURL(cfg.Telegram.Webhook.PublicURL)),
		paymentService:   paymentService,
		robokassaService: robokassaService,
	}
	appCtx.adminHandler = admin.NewHandler(st, cfg.Security.AdminToken, cfg.Telegram.BotUsername, tgClient, func(ctx context.Context, subscriptionID int64) (admin.RebillResult, error) {
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
	u.Path = strings.TrimRight(u.Path, "/")
	return strings.TrimRight(u.String(), "/")
}
