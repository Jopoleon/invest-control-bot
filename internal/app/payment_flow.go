package app

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/go-telegram/bot/models"
)

func (a *application) activateSuccessfulPayment(ctx context.Context, paymentRow domain.Payment, providerPaymentID string, now time.Time) {
	paymentMarkedNow := false
	effectivePaidAt := now
	if paymentRow.Status != domain.PaymentStatusPaid {
		updated, err := a.store.UpdatePaymentPaid(ctx, paymentRow.ID, providerPaymentID, now)
		if err != nil {
			slog.Error("update payment status failed", "error", err, "payment_id", paymentRow.ID)
			return
		}
		if updated {
			slog.Info("payment marked as paid", "payment_id", paymentRow.ID, "provider_payment_id", providerPaymentID)
			effectivePaidAt = now
			paymentMarkedNow = true
		} else {
			latestPayment, found, loadErr := a.store.GetPaymentByToken(ctx, paymentRow.Token)
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
	connector, connectorExists, err := a.store.GetConnector(ctx, paymentRow.ConnectorID)
	if err != nil {
		slog.Error("load connector for subscription failed", "error", err, "connector_id", paymentRow.ConnectorID)
	} else if connectorExists && connector.PeriodDays > 0 {
		periodDays = connector.PeriodDays
	}

	startAt := effectivePaidAt
	if latestSub, found, err := a.store.GetLatestSubscriptionByUserConnector(ctx, paymentRow.TelegramID, paymentRow.ConnectorID); err != nil {
		slog.Error("load latest subscription failed", "error", err, "telegram_id", paymentRow.TelegramID, "connector_id", paymentRow.ConnectorID)
	} else if found && latestSub.Status == domain.SubscriptionStatusActive && latestSub.EndsAt.After(startAt) {
		startAt = latestSub.EndsAt
	}
	endsAt := startAt.AddDate(0, 0, periodDays)
	if err := a.store.UpsertSubscriptionByPayment(ctx, domain.Subscription{
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
	if err := a.store.SaveAuditEvent(ctx, domain.AuditEvent{
		TelegramID:  paymentRow.TelegramID,
		ConnectorID: paymentRow.ConnectorID,
		Action:      domain.AuditActionSubscriptionActivated,
		Details:     "payment_id=" + strconv.FormatInt(paymentRow.ID, 10),
		CreatedAt:   now,
	}); err != nil {
		slog.Error("save audit event failed", "error", err, "action", domain.AuditActionSubscriptionActivated)
	}

	if !paymentMarkedNow {
		return
	}
	channelURL := ""
	if connectorExists {
		channelURL = resolveConnectorChannelURL(connector.ChannelURL, connector.ChatID)
	}
	successText := fmt.Sprintf("✅ Оплата прошла успешно. Подписка активирована до %s.", endsAt.In(time.Local).Format("02.01.2006 15:04"))
	var keyboard *models.InlineKeyboardMarkup
	if channelURL != "" {
		successText += "\n\nНажмите кнопку ниже, чтобы перейти в канал и открыть кабинет."
		keyboard = &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "Перейти в канал", URL: channelURL}},
				{{Text: "Моя подписка", CallbackData: "menu:subscription"}},
			},
		}
	} else {
		keyboard = &models.InlineKeyboardMarkup{
			InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "Моя подписка", CallbackData: "menu:subscription"}},
			},
		}
	}
	if err := a.telegramClient.SendMessage(ctx, paymentRow.TelegramID, successText, keyboard); err != nil {
		slog.Error("send payment success message failed", "error", err, "telegram_id", paymentRow.TelegramID, "payment_id", paymentRow.ID)
		return
	}
	if err := a.store.SaveAuditEvent(ctx, domain.AuditEvent{
		TelegramID:  paymentRow.TelegramID,
		ConnectorID: paymentRow.ConnectorID,
		Action:      domain.AuditActionPaymentSuccessNotified,
		Details:     "payment_id=" + strconv.FormatInt(paymentRow.ID, 10),
		CreatedAt:   now,
	}); err != nil {
		slog.Error("save audit event failed", "error", err, "action", domain.AuditActionPaymentSuccessNotified)
	}
}
