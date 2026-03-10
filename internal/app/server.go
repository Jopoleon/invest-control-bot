package app

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Jopoleon/telega-bot-fedor/internal/admin"
	"github.com/Jopoleon/telega-bot-fedor/internal/bot"
	"github.com/Jopoleon/telega-bot-fedor/internal/config"
	"github.com/Jopoleon/telega-bot-fedor/internal/payment"
	"github.com/Jopoleon/telega-bot-fedor/internal/store"
	"github.com/Jopoleon/telega-bot-fedor/internal/telegram"
	"github.com/go-telegram/bot/models"
)

// Server owns HTTP server with admin endpoints, Telegram webhook and mock checkout routes.
type Server struct {
	httpServer *http.Server
}

// New builds fully wired HTTP server with current dependencies.
func New(cfg config.Config, st store.Store) (*Server, error) {
	tgClient, err := telegram.NewClient(cfg.Telegram.BotToken, cfg.Telegram.Webhook.SecretToken)
	if err != nil {
		return nil, fmt.Errorf("create telegram client: %w", err)
	}

	mockBaseURL := cfg.Payment.MockBaseURL
	if mockBaseURL == "" {
		mockBaseURL = cfg.Telegram.Webhook.PublicURL
	}
	var paymentService payment.Service
	switch cfg.Payment.Provider {
	case "", "mock":
		paymentService = payment.NewMockService(mockBaseURL)
	default:
		slog.Warn("payment provider is not implemented yet, fallback to mock", "provider", cfg.Payment.Provider)
		paymentService = payment.NewMockService(mockBaseURL)
	}

	botHandler := bot.NewHandler(st, tgClient, paymentService)
	adminHandler := admin.NewHandler(st, cfg.Security.AdminToken, cfg.Telegram.BotUsername)

	mux := http.NewServeMux()
	adminHandler.Register(mux)

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	// Mock checkout pages are temporary placeholders until real provider is selected.
	mux.HandleFunc("/mock/pay", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		token := r.URL.Query().Get("token")
		connectorID := r.URL.Query().Get("connector_id")
		userID := r.URL.Query().Get("user_id")
		amount := r.URL.Query().Get("amount_rub")
		_, _ = fmt.Fprintf(w, "<html><body style='font-family:sans-serif;background:#f5f7fb;'><div style='max-width:760px;margin:32px auto;background:#fff;border:1px solid #e5eaf2;border-radius:12px;padding:20px;'><h2>Mock Checkout</h2><p>Платежный шлюз пока в режиме заглушки.</p><p><b>Token:</b> %s<br><b>Connector:</b> %s<br><b>User:</b> %s<br><b>Amount:</b> %s RUB</p><a href='/mock/pay/success?token=%s' style='display:inline-block;padding:10px 14px;background:#111827;color:#fff;border-radius:8px;text-decoration:none;'>Имитировать успешную оплату</a></div></body></html>", token, connectorID, userID, amount, token)
	})
	mux.HandleFunc("/mock/pay/success", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		token := r.URL.Query().Get("token")
		_, _ = fmt.Fprintf(w, "<html><body style='font-family:sans-serif;background:#f5f7fb;'><div style='max-width:760px;margin:32px auto;background:#fff;border:1px solid #e5eaf2;border-radius:12px;padding:20px;'><h2>Mock Payment Succeeded</h2><p>Тестовая оплата подтверждена. Token: <b>%s</b></p><p>В проде здесь будет webhook от платежного провайдера.</p></div></body></html>", token)
	})

	mux.HandleFunc("/telegram/webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if cfg.Telegram.Webhook.SecretToken != "" && r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != cfg.Telegram.Webhook.SecretToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var update models.Update
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("invalid update payload"))
			return
		}
		logged := false
		// Keep raw update logs during active development to simplify webhook diagnostics.
		if update.Message != nil {
			raw, _ := json.Marshal(update.Message)
			slog.Debug("telegram update.message", "payload", string(raw))
			logged = true
		}
		if update.CallbackQuery != nil {
			raw, _ := json.Marshal(update.CallbackQuery.Message)
			slog.Debug("telegram update.callback_query.message", "payload", string(raw))
			logged = true
		}
		if update.ChannelPost != nil {
			raw, _ := json.Marshal(update.ChannelPost)
			slog.Debug("telegram update.channel_post", "payload", string(raw))
			logged = true
		}
		if update.EditedChannelPost != nil {
			raw, _ := json.Marshal(update.EditedChannelPost)
			slog.Debug("telegram update.edited_channel_post", "payload", string(raw))
			logged = true
		}
		if !logged {
			raw, _ := json.Marshal(update)
			slog.Debug("telegram update.raw", "payload", string(raw))
		}
		botHandler.HandleUpdate(r.Context(), &update)
		w.WriteHeader(http.StatusOK)
	})

	httpServer := &http.Server{
		Addr:         cfg.HTTP.Address,
		Handler:      loggingMiddleware(mux),
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	return &Server{httpServer: httpServer}, nil
}

// Run starts HTTP server and blocks until it stops.
func (s *Server) Run() error {
	return s.httpServer.ListenAndServe()
}

// loggingMiddleware logs basic request metadata for every incoming HTTP call.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("http request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
