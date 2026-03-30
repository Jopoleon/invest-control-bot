package main

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Jopoleon/invest-control-bot/internal/app"
	"github.com/Jopoleon/invest-control-bot/internal/bot"
	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/logger"
	"github.com/Jopoleon/invest-control-bot/internal/max"
	"github.com/Jopoleon/invest-control-bot/internal/payment"
)

// main runs local MAX long polling and dispatches updates into the shared bot core.
func main() {
	if _, err := logger.Init("info", ""); err != nil {
		slog.Error("bootstrap logger init failed", "error", err)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}
	if strings.TrimSpace(cfg.MAX.BotToken) == "" {
		slog.Error("MAX_BOT_TOKEN is required for cmd/max-poller")
		os.Exit(1)
	}
	effectiveLevel, err := logger.Init(cfg.Logging.Level, cfg.Logging.FilePath)
	if err != nil {
		slog.Error("logger init with file failed", "error", err, "file_path", cfg.Logging.FilePath)
		os.Exit(1)
	}
	slog.Info("config loaded", "effective_log_level", effectiveLevel, "log_file_path", cfg.Logging.FilePath)

	st, cleanup, err := app.OpenStore(cfg)
	if err != nil {
		slog.Error("init store failed", "error", err)
		os.Exit(1)
	}
	defer cleanup()

	paymentService := buildPaymentService(cfg)
	maxClient := max.NewClient(cfg.MAX.BotToken, nil)
	botInfo, err := maxClient.Ping(context.Background())
	if err != nil {
		slog.Error("ping MAX api failed", "error", err)
		os.Exit(1)
	}
	slog.Info("MAX api ping ok", "bot_id", botInfo.UserID, "bot_username", botInfo.Username, "bot_name", firstNonEmpty(botInfo.FirstName, botInfo.Name))
	maxSender := max.NewSender(maxClient)
	handler := bot.NewHandler(
		st,
		maxSender,
		paymentService,
		cfg.Payment.Provider == "robokassa" && cfg.Payment.Robokassa.RecurringEnabled,
		resolvePublicBaseURL(cfg),
		cfg.Security.EncryptionKey,
	)
	adapter := max.NewAdapter(handler)
	poller := &max.Poller{
		Client:     maxClient,
		Limit:      cfg.MAX.Polling.Limit,
		TimeoutSec: cfg.MAX.Polling.TimeoutSec,
		Types:      cfg.MAX.Polling.Types,
	}

	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("max poller started", "types", cfg.MAX.Polling.Types, "timeout_sec", cfg.MAX.Polling.TimeoutSec, "limit", cfg.MAX.Polling.Limit)
	err = poller.Run(runCtx, func(ctx context.Context, update max.Update) error {
		slog.Debug("max update received",
			"update_type", update.UpdateType,
			"chat_id", update.ChatID,
			"sender_user_id", senderUserID(update),
			"recipient_user_id", recipientUserID(update),
			"recipient_chat_id", recipientChatID(update),
			"callback_id", callbackID(update),
			"payload", payloadValue(update),
			"text", messageText(update),
			"message_mid", messageMID(update),
			"raw", rawUpdate(update),
		)
		adapter.Dispatch(ctx, update)
		return nil
	})
	if err != nil && err != context.Canceled {
		slog.Error("max poller stopped", "error", err)
		os.Exit(1)
	}
}

func senderUserID(update max.Update) int64 {
	if update.Message != nil && update.Message.Sender != nil {
		return update.Message.Sender.UserID
	}
	if update.Callback != nil && update.Callback.Message != nil && update.Callback.Message.Sender != nil {
		return update.Callback.Message.Sender.UserID
	}
	if update.User != nil {
		return update.User.UserID
	}
	if update.Callback != nil && update.Callback.User != nil {
		return update.Callback.User.UserID
	}
	return 0
}

func recipientUserID(update max.Update) int64 {
	if update.Message != nil && update.Message.Recipient != nil {
		return update.Message.Recipient.UserID
	}
	if update.Callback != nil && update.Callback.Message != nil && update.Callback.Message.Recipient != nil {
		return update.Callback.Message.Recipient.UserID
	}
	return 0
}

func recipientChatID(update max.Update) int64 {
	if update.Message != nil && update.Message.Recipient != nil {
		return update.Message.Recipient.ChatID
	}
	if update.Callback != nil && update.Callback.Message != nil && update.Callback.Message.Recipient != nil {
		return update.Callback.Message.Recipient.ChatID
	}
	return 0
}

func callbackID(update max.Update) string {
	if update.Callback == nil {
		return ""
	}
	return update.Callback.CallbackID
}

func payloadValue(update max.Update) string {
	switch {
	case update.Callback != nil && strings.TrimSpace(update.Callback.Payload) != "":
		return update.Callback.Payload
	case update.Callback != nil:
		return update.Callback.Data
	case strings.TrimSpace(update.StartPayload) != "":
		return update.StartPayload
	default:
		return update.Payload
	}
}

func messageText(update max.Update) string {
	switch {
	case update.Message != nil && update.Message.Body != nil && strings.TrimSpace(update.Message.Body.Text) != "":
		return update.Message.Body.Text
	case update.Message != nil:
		return update.Message.Text
	case update.Callback != nil && update.Callback.Message != nil && update.Callback.Message.Body != nil && strings.TrimSpace(update.Callback.Message.Body.Text) != "":
		return update.Callback.Message.Body.Text
	case update.Callback != nil && update.Callback.Message != nil:
		return update.Callback.Message.Text
	default:
		return ""
	}
}

func messageMID(update max.Update) string {
	switch {
	case update.Message != nil && update.Message.Body != nil:
		return update.Message.Body.MID
	case update.Callback != nil && update.Callback.Message != nil && update.Callback.Message.Body != nil:
		return update.Callback.Message.Body.MID
	default:
		return ""
	}
}

func rawUpdate(update max.Update) string {
	return string(update.Raw)
}

func buildPaymentService(cfg config.Config) payment.Service {
	mockBaseURL := strings.TrimSpace(cfg.Payment.MockBaseURL)
	if mockBaseURL == "" {
		mockBaseURL = resolvePublicBaseURL(cfg)
	}

	switch cfg.Payment.Provider {
	case "", "mock":
		return payment.NewMockService(mockBaseURL)
	case "robokassa":
		return payment.NewRobokassaService(payment.RobokassaConfig{
			MerchantLogin: cfg.Payment.Robokassa.MerchantLogin,
			Password1:     cfg.Payment.Robokassa.Password1,
			Password2:     cfg.Payment.Robokassa.Password2,
			IsTest:        cfg.Payment.Robokassa.IsTestMode,
			BaseURL:       cfg.Payment.Robokassa.CheckoutURL,
			RebillURL:     cfg.Payment.Robokassa.RebillURL,
		})
	default:
		slog.Warn("payment provider is not implemented yet, fallback to mock", "provider", cfg.Payment.Provider)
		return payment.NewMockService(mockBaseURL)
	}
}

func resolvePublicBaseURL(cfg config.Config) string {
	switch {
	case strings.TrimSpace(cfg.MAX.Webhook.PublicURL) != "":
		return trimWebhookPath(cfg.MAX.Webhook.PublicURL, "/max/webhook")
	case strings.TrimSpace(cfg.Telegram.Webhook.PublicURL) != "":
		return trimWebhookPath(cfg.Telegram.Webhook.PublicURL, "/telegram/webhook")
	default:
		return ""
	}
}

func trimWebhookPath(raw, suffix string) string {
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
	u.Path = strings.TrimSuffix(u.Path, suffix)
	u.Path = strings.TrimRight(u.Path, "/")
	return strings.TrimRight(u.String(), "/")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
