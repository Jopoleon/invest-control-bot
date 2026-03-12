package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/admin"
	"github.com/Jopoleon/telega-bot-fedor/internal/bot"
	"github.com/Jopoleon/telega-bot-fedor/internal/config"
	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
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
		})
		paymentService = robokassaService
	default:
		slog.Warn("payment provider is not implemented yet, fallback to mock", "provider", cfg.Payment.Provider)
		paymentService = payment.NewMockService(mockBaseURL)
	}

	botHandler := bot.NewHandler(st, tgClient, paymentService)
	adminHandler := admin.NewHandler(st, cfg.Security.AdminToken, cfg.Telegram.BotUsername)

	mux := http.NewServeMux()
	adminHandler.Register(mux)

	activateSuccessfulPayment := func(ctx context.Context, paymentRow domain.Payment, providerPaymentID string, now time.Time) {
		paymentMarkedNow := false
		effectivePaidAt := now
		if paymentRow.Status != domain.PaymentStatusPaid {
			if err := st.UpdatePaymentPaid(ctx, paymentRow.ID, providerPaymentID, now); err != nil {
				slog.Error("update payment status failed", "error", err, "payment_id", paymentRow.ID)
				return
			}
			slog.Info("payment marked as paid", "payment_id", paymentRow.ID, "provider_payment_id", providerPaymentID)
			effectivePaidAt = now
			paymentMarkedNow = true
		} else if paymentRow.PaidAt != nil {
			effectivePaidAt = *paymentRow.PaidAt
		}

		periodDays := 30
		connector, connectorExists, err := st.GetConnector(ctx, paymentRow.ConnectorID)
		if err != nil {
			slog.Error("load connector for subscription failed", "error", err, "connector_id", paymentRow.ConnectorID)
		} else if connectorExists && connector.PeriodDays > 0 {
			periodDays = connector.PeriodDays
		}
		if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
			TelegramID:  paymentRow.TelegramID,
			ConnectorID: paymentRow.ConnectorID,
			PaymentID:   paymentRow.ID,
			Status:      domain.SubscriptionStatusActive,
			StartsAt:    effectivePaidAt,
			EndsAt:      effectivePaidAt.AddDate(0, 0, periodDays),
			CreatedAt:   effectivePaidAt,
			UpdatedAt:   now,
		}); err != nil {
			slog.Error("upsert subscription failed", "error", err, "payment_id", paymentRow.ID)
			return
		}
		slog.Info("subscription activated", "payment_id", paymentRow.ID, "telegram_id", paymentRow.TelegramID, "connector_id", paymentRow.ConnectorID, "ends_at", effectivePaidAt.AddDate(0, 0, periodDays))
		if err := st.SaveAuditEvent(ctx, domain.AuditEvent{
			TelegramID:  paymentRow.TelegramID,
			ConnectorID: paymentRow.ConnectorID,
			Action:      "subscription_activated",
			Details:     "payment_id=" + strconv.FormatInt(paymentRow.ID, 10),
			CreatedAt:   now,
		}); err != nil {
			slog.Error("save audit event failed", "error", err, "action", "subscription_activated")
		}

		if !paymentMarkedNow {
			return
		}
		channelURL := ""
		if connectorExists {
			channelURL = resolveConnectorChannelURL(connector.ChannelURL, connector.ChatID)
		}
		successText := fmt.Sprintf(
			"✅ Оплата прошла успешно. Подписка активирована до %s.",
			effectivePaidAt.AddDate(0, 0, periodDays).In(time.Local).Format("02.01.2006 15:04"),
		)
		var keyboard *models.InlineKeyboardMarkup
		if channelURL != "" {
			successText += "\n\nНажмите кнопку ниже, чтобы перейти в канал и открыть кабинет."
			keyboard = &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					{
						{Text: "Перейти в канал", URL: channelURL},
					},
					{
						{Text: "Моя подписка", CallbackData: "menu:subscription"},
					},
				},
			}
		} else {
			keyboard = &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{
					{
						{Text: "Моя подписка", CallbackData: "menu:subscription"},
					},
				},
			}
		}
		if err := tgClient.SendMessage(ctx, paymentRow.TelegramID, successText, keyboard); err != nil {
			slog.Error("send payment success message failed", "error", err, "telegram_id", paymentRow.TelegramID, "payment_id", paymentRow.ID)
		} else {
			if err := st.SaveAuditEvent(ctx, domain.AuditEvent{
				TelegramID:  paymentRow.TelegramID,
				ConnectorID: paymentRow.ConnectorID,
				Action:      "payment_success_notified",
				Details:     "payment_id=" + strconv.FormatInt(paymentRow.ID, 10),
				CreatedAt:   now,
			}); err != nil {
				slog.Error("save audit event failed", "error", err, "action", "payment_success_notified")
			}
		}
	}

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	// Mock checkout pages are temporary placeholders until real provider is selected.
	mux.HandleFunc("/mock/pay", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		token := r.URL.Query().Get("token")
		connectorIDRaw := r.URL.Query().Get("connector_id")
		var connectorID int64
		if parsed, err := strconv.ParseInt(connectorIDRaw, 10, 64); err == nil && parsed > 0 {
			connectorID = parsed
		}
		userID := r.URL.Query().Get("user_id")
		amount := r.URL.Query().Get("amount_rub")
		if tgID, err := strconv.ParseInt(userID, 10, 64); err == nil && tgID > 0 {
			if err := st.SaveAuditEvent(r.Context(), domain.AuditEvent{
				TelegramID:  tgID,
				ConnectorID: connectorID,
				Action:      "mock_checkout_opened",
				Details:     "token=" + token,
				CreatedAt:   time.Now().UTC(),
			}); err != nil {
				slog.Error("save audit event failed", "error", err, "action", "mock_checkout_opened")
			}
		}
		_, _ = fmt.Fprintf(w, "<html><body style='font-family:sans-serif;background:#f5f7fb;'><div style='max-width:760px;margin:32px auto;background:#fff;border:1px solid #e5eaf2;border-radius:12px;padding:20px;'><h2>Mock Checkout</h2><p>Платежный шлюз пока в режиме заглушки.</p><p><b>Token:</b> %s<br><b>Connector:</b> %d<br><b>User:</b> %s<br><b>Amount:</b> %s RUB</p><a href='/mock/pay/success?token=%s&connector_id=%d&user_id=%s' style='display:inline-block;padding:10px 14px;background:#111827;color:#fff;border-radius:8px;text-decoration:none;'>Имитировать успешную оплату</a></div></body></html>", token, connectorID, userID, amount, token, connectorID, userID)
	})
	mux.HandleFunc("/mock/pay/success", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		token := r.URL.Query().Get("token")
		connectorIDRaw := r.URL.Query().Get("connector_id")
		var connectorID int64
		if parsed, err := strconv.ParseInt(connectorIDRaw, 10, 64); err == nil && parsed > 0 {
			connectorID = parsed
		}
		userID := r.URL.Query().Get("user_id")
		now := time.Now().UTC()
		paymentRow, ok, err := st.GetPaymentByToken(r.Context(), token)
		if err != nil {
			slog.Error("load payment by token failed", "error", err, "token", token)
		}
		if ok {
			connectorID = paymentRow.ConnectorID
			userID = strconv.FormatInt(paymentRow.TelegramID, 10)
			activateSuccessfulPayment(r.Context(), paymentRow, "mock:"+token, now)
		}
		if tgID, err := strconv.ParseInt(userID, 10, 64); err == nil && tgID > 0 {
			if err := st.SaveAuditEvent(r.Context(), domain.AuditEvent{
				TelegramID:  tgID,
				ConnectorID: connectorID,
				Action:      "mock_payment_success",
				Details:     "token=" + token,
				CreatedAt:   now,
			}); err != nil {
				slog.Error("save audit event failed", "error", err, "action", "mock_payment_success")
			}
		}
		_, _ = fmt.Fprintf(w, "<html><body style='font-family:sans-serif;background:#f5f7fb;'><div style='max-width:760px;margin:32px auto;background:#fff;border:1px solid #e5eaf2;border-radius:12px;padding:20px;'><h2>Mock Payment Succeeded</h2><p>Тестовая оплата подтверждена. Token: <b>%s</b></p><p>В проде здесь будет webhook от платежного провайдера.</p></div></body></html>", token)
	})
	mux.HandleFunc("/payment/result", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if robokassaService == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		outSum := firstNonEmpty(r.FormValue("OutSum"), r.FormValue("out_summ"))
		invID := firstNonEmpty(r.FormValue("InvId"), r.FormValue("InvID"), r.FormValue("InvoiceID"), r.FormValue("invoice_id"))
		signature := firstNonEmpty(r.FormValue("SignatureValue"), r.FormValue("signaturevalue"))
		if outSum == "" || invID == "" || signature == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !robokassaService.VerifyResultSignature(outSum, invID, signature) {
			slog.Warn("robokassa result signature mismatch", "inv_id", invID)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		paymentRow, ok, err := st.GetPaymentByToken(r.Context(), invID)
		if err != nil {
			slog.Error("load payment by robokassa inv_id failed", "error", err, "inv_id", invID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if !ok {
			slog.Warn("payment not found for robokassa inv_id", "inv_id", invID)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		activateSuccessfulPayment(r.Context(), paymentRow, "robokassa:"+invID, time.Now().UTC())
		if err := st.SaveAuditEvent(r.Context(), domain.AuditEvent{
			TelegramID:  paymentRow.TelegramID,
			ConnectorID: paymentRow.ConnectorID,
			Action:      "robokassa_result_received",
			Details:     "inv_id=" + invID + ";out_sum=" + outSum,
			CreatedAt:   time.Now().UTC(),
		}); err != nil {
			slog.Error("save audit event failed", "error", err, "action", "robokassa_result_received")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK" + invID))
	})
	mux.HandleFunc("/payment/success", func(w http.ResponseWriter, r *http.Request) {
		if robokassaService == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		outSum := firstNonEmpty(r.FormValue("OutSum"), r.FormValue("out_summ"))
		invID := firstNonEmpty(r.FormValue("InvId"), r.FormValue("InvID"), r.FormValue("InvoiceID"), r.FormValue("invoice_id"))
		signature := firstNonEmpty(r.FormValue("SignatureValue"), r.FormValue("signaturevalue"))
		if outSum != "" && invID != "" && signature != "" && !robokassaService.VerifySuccessSignature(outSum, invID, signature) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		botURL := buildBotChatURL(cfg.Telegram.BotUsername)
		channelURL := ""
		if invID != "" {
			if paymentRow, ok, err := st.GetPaymentByToken(r.Context(), invID); err == nil && ok {
				if connector, found, err := st.GetConnector(r.Context(), paymentRow.ConnectorID); err == nil && found {
					channelURL = resolveConnectorChannelURL(connector.ChannelURL, connector.ChatID)
				}
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var actionButtons strings.Builder
		if botURL != "" {
			actionButtons.WriteString(`<a class="btn" href="` + botURL + `">Открыть бота</a>`)
		}
		if channelURL != "" {
			actionButtons.WriteString(`<a class="btn secondary" href="` + channelURL + `">Открыть канал</a>`)
		}
		if actionButtons.Len() == 0 {
			actionButtons.WriteString(`<a class="btn" href="https://t.me">Открыть Telegram</a>`)
		}
		_, _ = w.Write([]byte(`<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Оплата успешна</title><style>
body{font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;background:#f4f7fb;margin:0;padding:24px}
.card{max-width:620px;margin:36px auto;background:#fff;border:1px solid #e5eaf2;border-radius:14px;padding:24px}
h2{margin:0 0 10px}.muted{color:#5b6472}.row{display:flex;gap:10px;flex-wrap:wrap;margin-top:18px}
.btn{display:inline-block;padding:10px 14px;border-radius:10px;background:#111827;color:#fff;text-decoration:none}
.btn.secondary{background:#4b5563}
</style></head><body><div class="card">
<h2>Оплата успешно завершена</h2>
<p class="muted">Платеж подтвержден. Подписка активируется автоматически и в боте придет сообщение с деталями.</p>
<div class="row">` + actionButtons.String() + `</div>
</div></body></html>`))
	})
	mux.HandleFunc("/payment/fail", func(w http.ResponseWriter, r *http.Request) {
		if robokassaService == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		botURL := buildBotChatURL(cfg.Telegram.BotUsername)
		link := "https://t.me"
		if botURL != "" {
			link = botURL
		}
		_, _ = w.Write([]byte(`<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Оплата не завершена</title><style>
body{font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;background:#f4f7fb;margin:0;padding:24px}
.card{max-width:620px;margin:36px auto;background:#fff;border:1px solid #e5eaf2;border-radius:14px;padding:24px}
h2{margin:0 0 10px}.muted{color:#5b6472}.btn{display:inline-block;padding:10px 14px;border-radius:10px;background:#111827;color:#fff;text-decoration:none;margin-top:14px}
</style></head><body><div class="card">
<h2>Оплата не завершена</h2>
<p class="muted">Платеж был отменен или не прошел. Вернитесь в бота и попробуйте снова.</p>
<a class="btn" href="` + link + `">Вернуться в бота</a>
</div></body></html>`))
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
	startSubscriptionLifecycleWorker(st, tgClient, cfg.Telegram.BotUsername)

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

// resolveConnectorChannelURL returns explicit channel URL if provided, otherwise derives fallback from chat_id.
func resolveConnectorChannelURL(channelURL, chatID string) string {
	explicit := strings.TrimSpace(channelURL)
	if explicit != "" {
		if normalized := normalizeTelegramPublicLink(explicit); normalized != "" {
			return normalized
		}
	}
	return buildTelegramChatURL(chatID)
}

// buildTelegramChatURL builds best-effort fallback link from stored connector chat_id.
func buildTelegramChatURL(chatID string) string {
	raw := strings.TrimSpace(chatID)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "@") && len(raw) > 1 {
		return "https://t.me/" + strings.TrimPrefix(raw, "@")
	}
	if _, err := strconv.ParseInt(raw, 10, 64); err != nil {
		return "https://t.me/" + strings.TrimPrefix(raw, "@")
	}
	normalized := strings.TrimPrefix(raw, "-")
	normalized = strings.TrimPrefix(normalized, "100")
	if normalized == "" {
		return ""
	}
	return "https://t.me/c/" + normalized
}

func buildBotChatURL(botUsername string) string {
	raw := strings.TrimSpace(strings.TrimPrefix(botUsername, "@"))
	if raw == "" {
		return ""
	}
	return "https://t.me/" + raw
}

func normalizeTelegramPublicLink(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	v = strings.TrimPrefix(v, "https://")
	v = strings.TrimPrefix(v, "http://")
	v = strings.TrimPrefix(v, "t.me/")
	v = strings.TrimPrefix(v, "telegram.me/")
	v = strings.TrimPrefix(v, "@")
	v = strings.TrimPrefix(v, "/")
	if v == "" || strings.Contains(v, " ") {
		return ""
	}
	return "https://t.me/" + v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
