package app

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
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
	"github.com/go-co-op/gocron/v2"
	"github.com/go-telegram/bot/models"
)

// Server owns HTTP server with admin endpoints, Telegram webhook and mock checkout routes.
type Server struct {
	httpServer          *http.Server
	lifecycleScheduler  gocron.Scheduler
	lifecycleRunOnStart func()
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
			RebillURL:     cfg.Payment.Robokassa.RebillURL,
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
			updated, err := st.UpdatePaymentPaid(ctx, paymentRow.ID, providerPaymentID, now)
			if err != nil {
				slog.Error("update payment status failed", "error", err, "payment_id", paymentRow.ID)
				return
			}
			if updated {
				slog.Info("payment marked as paid", "payment_id", paymentRow.ID, "provider_payment_id", providerPaymentID)
				effectivePaidAt = now
				paymentMarkedNow = true
			} else {
				latestPayment, found, loadErr := st.GetPaymentByToken(ctx, paymentRow.Token)
				if loadErr != nil {
					slog.Error("reload payment failed", "error", loadErr, "payment_id", paymentRow.ID)
				} else if found && latestPayment.PaidAt != nil {
					effectivePaidAt = *latestPayment.PaidAt
					paymentRow = latestPayment
				}
			}
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
		startAt := effectivePaidAt
		if latestSub, found, err := st.GetLatestSubscriptionByUserConnector(ctx, paymentRow.TelegramID, paymentRow.ConnectorID); err != nil {
			slog.Error("load latest subscription failed", "error", err, "telegram_id", paymentRow.TelegramID, "connector_id", paymentRow.ConnectorID)
		} else if found && latestSub.Status == domain.SubscriptionStatusActive && latestSub.EndsAt.After(startAt) {
			startAt = latestSub.EndsAt
		}
		endsAt := startAt.AddDate(0, 0, periodDays)
		if err := st.UpsertSubscriptionByPayment(ctx, domain.Subscription{
			TelegramID:     paymentRow.TelegramID,
			ConnectorID:    paymentRow.ConnectorID,
			PaymentID:      paymentRow.ID,
			Status:         domain.SubscriptionStatusActive,
			AutoPayEnabled: paymentRow.AutoPayEnabled,
			StartsAt:       startAt,
			EndsAt:         endsAt,
			CreatedAt:      startAt,
			UpdatedAt:      now,
		}); err != nil {
			slog.Error("upsert subscription failed", "error", err, "payment_id", paymentRow.ID)
			return
		}
		slog.Info("subscription activated", "payment_id", paymentRow.ID, "telegram_id", paymentRow.TelegramID, "connector_id", paymentRow.ConnectorID, "starts_at", startAt, "ends_at", endsAt)
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
			endsAt.In(time.Local).Format("02.01.2006 15:04"),
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
		successURL := fmt.Sprintf("/mock/pay/success?token=%s&connector_id=%d&user_id=%s", token, connectorID, userID)
		renderAppTemplate(w, "mock_pay.html", mockCheckoutPageData{
			Token:       token,
			ConnectorID: connectorID,
			UserID:      userID,
			Amount:      amount,
			SuccessURL:  successURL,
		})
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
		renderAppTemplate(w, "mock_pay_success.html", mockPaymentSuccessPageData{Token: token})
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
		amountKopeks, parseErr := parseRobokassaAmountToKopeks(outSum)
		if parseErr != nil {
			slog.Warn("robokassa outsum parse failed", "inv_id", invID, "out_sum", outSum, "error", parseErr)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		expectedKopeks := paymentRow.AmountRUB * 100
		if amountKopeks != expectedKopeks {
			slog.Warn("robokassa outsum mismatch", "inv_id", invID, "expected_kopeks", expectedKopeks, "actual_kopeks", amountKopeks)
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
		renderPaymentPage(w, paymentPageData{
			Title:   "Оплата успешно завершена",
			Message: "Платеж подтвержден. Подписка активируется автоматически, а в боте придет сообщение с деталями.",
			Actions: []paymentPageAction{
				{Label: "Открыть бота", URL: botURL},
				{Label: "Открыть канал", URL: channelURL, Secondary: true},
				{Label: "Открыть Telegram", URL: "https://t.me"},
			},
		})
	})
	mux.HandleFunc("/payment/fail", func(w http.ResponseWriter, r *http.Request) {
		if robokassaService == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = r.ParseForm()
		invID := firstNonEmpty(
			r.FormValue("InvId"),
			r.FormValue("InvID"),
			r.FormValue("InvoiceID"),
			r.FormValue("invoice_id"),
			r.URL.Query().Get("InvId"),
			r.URL.Query().Get("InvID"),
			r.URL.Query().Get("InvoiceID"),
			r.URL.Query().Get("invoice_id"),
		)
		if invID != "" {
			if paymentRow, ok, err := st.GetPaymentByToken(r.Context(), invID); err == nil && ok {
				if updated, err := st.UpdatePaymentFailed(r.Context(), paymentRow.ID, "robokassa:"+invID, time.Now().UTC()); err != nil {
					slog.Error("mark payment failed failed", "error", err, "payment_id", paymentRow.ID)
				} else if updated {
					_ = st.SaveAuditEvent(r.Context(), domain.AuditEvent{
						TelegramID:  paymentRow.TelegramID,
						ConnectorID: paymentRow.ConnectorID,
						Action:      "payment_failed",
						Details:     "payment_id=" + strconv.FormatInt(paymentRow.ID, 10),
						CreatedAt:   time.Now().UTC(),
					})
				}
			}
		}
		botURL := buildBotChatURL(cfg.Telegram.BotUsername)
		if botURL == "" {
			botURL = "https://t.me"
		}
		renderPaymentPage(w, paymentPageData{
			Title:   "Оплата не завершена",
			Message: "Платеж был отменен или не прошел. Вернитесь в бота и попробуйте снова.",
			Actions: []paymentPageAction{
				{Label: "Вернуться в бота", URL: botURL},
			},
		})
	})
	mux.HandleFunc("/payment/rebill", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if robokassaService == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if !authorizedAdminRequest(r, cfg.Security.AdminToken) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		subscriptionID, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("subscription_id")), 10, 64)
		if err != nil || subscriptionID <= 0 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("subscription_id is required"))
			return
		}
		subscription, found, err := st.GetSubscriptionByID(r.Context(), subscriptionID)
		if err != nil {
			slog.Error("get subscription for rebill failed", "error", err, "subscription_id", subscriptionID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if !subscription.AutoPayEnabled {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("autopay is disabled for subscription"))
			return
		}
		parentPayment, found, err := st.GetPaymentByID(r.Context(), subscription.PaymentID)
		if err != nil {
			slog.Error("get parent payment for rebill failed", "error", err, "payment_id", subscription.PaymentID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if !found || strings.TrimSpace(parentPayment.Token) == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("parent payment is missing token"))
			return
		}
		connector, found, err := st.GetConnector(r.Context(), subscription.ConnectorID)
		if err != nil {
			slog.Error("get connector for rebill failed", "error", err, "connector_id", subscription.ConnectorID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if !found {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("connector not found"))
			return
		}
		invoiceID := generateInvoiceID()
		if err := robokassaService.CreateRebill(r.Context(), payment.RebillRequest{
			InvoiceID:         invoiceID,
			PreviousInvoiceID: parentPayment.Token,
			AmountRUB:         connector.PriceRUB,
			Description:       connector.Name,
		}); err != nil {
			slog.Error("robokassa rebill request failed", "error", err, "subscription_id", subscriptionID, "invoice_id", invoiceID)
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("rebill request failed"))
			return
		}
		now := time.Now().UTC()
		if err := st.CreatePayment(r.Context(), domain.Payment{
			Provider:          "robokassa",
			Status:            domain.PaymentStatusPending,
			Token:             invoiceID,
			TelegramID:        subscription.TelegramID,
			ConnectorID:       subscription.ConnectorID,
			AmountRUB:         connector.PriceRUB,
			AutoPayEnabled:    true,
			ProviderPaymentID: "rebill_parent:" + parentPayment.Token,
			CreatedAt:         now,
			UpdatedAt:         now,
		}); err != nil {
			slog.Error("create rebill payment failed", "error", err, "invoice_id", invoiceID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := st.SaveAuditEvent(r.Context(), domain.AuditEvent{
			TelegramID:  subscription.TelegramID,
			ConnectorID: subscription.ConnectorID,
			Action:      "rebill_requested",
			Details:     "subscription_id=" + strconv.FormatInt(subscriptionID, 10) + ";invoice_id=" + invoiceID + ";parent=" + parentPayment.Token,
			CreatedAt:   now,
		}); err != nil {
			slog.Error("save audit event failed", "error", err, "action", "rebill_requested")
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"ok":true,"invoice_id":"` + invoiceID + `"}`))
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
	lifecycleScheduler, err := newSubscriptionLifecycleScheduler(st, tgClient, cfg.Telegram.BotUsername)
	if err != nil {
		return nil, fmt.Errorf("create subscription lifecycle scheduler: %w", err)
	}

	return &Server{
		httpServer:         httpServer,
		lifecycleScheduler: lifecycleScheduler,
		lifecycleRunOnStart: func() {
			runSubscriptionLifecyclePass(context.Background(), st, tgClient, cfg.Telegram.BotUsername)
		},
	}, nil
}

// Run starts HTTP server and blocks until it stops.
func (s *Server) Run(ctx context.Context) error {
	if s.lifecycleScheduler != nil {
		s.lifecycleRunOnStart()
		s.lifecycleScheduler.Start()
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.Shutdown(shutdownCtx); err != nil {
			return err
		}
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// Shutdown stops HTTP handling and background schedulers.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.lifecycleScheduler != nil {
		if err := s.lifecycleScheduler.Shutdown(); err != nil {
			return err
		}
	}
	return s.httpServer.Shutdown(ctx)
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

func authorizedAdminRequest(r *http.Request, adminToken string) bool {
	adminToken = strings.TrimSpace(adminToken)
	if adminToken == "" {
		return true
	}
	if strings.TrimSpace(r.Header.Get("X-Admin-Token")) == adminToken {
		return true
	}
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")) == adminToken
	}
	return false
}

func generateInvoiceID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	v := int64(binary.BigEndian.Uint64(b[:]) & 0x7fffffffffffffff)
	if v < 1_000_000_000 {
		v += 1_000_000_000
	}
	return strconv.FormatInt(v, 10)
}

type paymentPageAction struct {
	Label     string
	URL       string
	Secondary bool
}

type paymentPageData struct {
	Title   string
	Message string
	Actions []paymentPageAction
}

func renderPaymentPage(w http.ResponseWriter, data paymentPageData) {
	renderAppTemplate(w, "payment_status.html", data)
}

func parseRobokassaAmountToKopeks(raw string) (int64, error) {
	value := strings.TrimSpace(strings.ReplaceAll(raw, ",", "."))
	if value == "" {
		return 0, fmt.Errorf("amount is empty")
	}
	if strings.HasPrefix(value, "-") {
		return 0, fmt.Errorf("negative amount")
	}
	parts := strings.SplitN(value, ".", 2)
	rubles, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || rubles < 0 {
		return 0, fmt.Errorf("invalid rubles part")
	}
	var kopeks int64
	if len(parts) == 2 {
		fraction := parts[1]
		if fraction == "" {
			return 0, fmt.Errorf("invalid fractional part")
		}
		if len(fraction) > 2 {
			if strings.Trim(fraction[2:], "0") != "" {
				return 0, fmt.Errorf("too many fractional digits")
			}
			fraction = fraction[:2]
		}
		if len(fraction) == 1 {
			fraction += "0"
		}
		kopeks, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil || kopeks < 0 {
			return 0, fmt.Errorf("invalid kopeks part")
		}
	}
	return rubles*100 + kopeks, nil
}

type mockCheckoutPageData struct {
	Token       string
	ConnectorID int64
	UserID      string
	Amount      string
	SuccessURL  string
}

type mockPaymentSuccessPageData struct {
	Token string
}
