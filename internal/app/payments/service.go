package payments

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	"github.com/Jopoleon/invest-control-bot/internal/store"
)

// Service owns payment-side business rules that do not need to live on HTTP
// handlers directly: payment activation and recurring failure notifications.
//
// The root app package injects the few cross-cutting dependencies that still
// connect this logic to messenger delivery, audit creation, and channel URL
// resolution, so the service stays testable without the full application
// object.
type Service struct {
	Store                   store.Store
	TelegramBotUsername     string
	SuccessChannelHint      string
	OpenChannelActionLabel  string
	MySubscriptionAction    string
	FailedRecurringText     string
	FailedRecurringButton   string
	PaymentSuccessMessage   func(time.Time) string
	BuildTelegramAccessLink func(context.Context, int64, domain.Connector) (string, error)
	ResolveConnectorChannel func(string, string) string
	SendUserNotification    func(context.Context, int64, string, messenger.OutgoingMessage) error
	BuildTargetAuditEvent   func(context.Context, int64, string, int64, string, string, time.Time) domain.AuditEvent
}

func (s *Service) ActivateSuccessfulPayment(ctx context.Context, paymentRow domain.Payment, providerPaymentID string, now time.Time) {
	paymentMarkedNow := false
	effectivePaidAt := now
	if paymentRow.Status != domain.PaymentStatusPaid {
		updated, err := s.Store.UpdatePaymentPaid(ctx, paymentRow.ID, providerPaymentID, now)
		if err != nil {
			slog.Error("update payment status failed", "error", err, "payment_id", paymentRow.ID)
			return
		}
		if updated {
			slog.Info("payment marked as paid", "payment_id", paymentRow.ID, "provider_payment_id", providerPaymentID)
			effectivePaidAt = now
			paymentMarkedNow = true
		} else {
			latestPayment, found, loadErr := s.Store.GetPaymentByToken(ctx, paymentRow.Token)
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

	endsAt := effectivePaidAt.AddDate(0, 0, 30)
	connector, connectorExists, err := s.Store.GetConnector(ctx, paymentRow.ConnectorID)
	if err != nil {
		slog.Error("load connector for subscription failed", "error", err, "connector_id", paymentRow.ConnectorID)
	} else if connectorExists {
		endsAt = connector.SubscriptionEndsAt(effectivePaidAt)
	}

	startAt := effectivePaidAt
	if latestSub, found, err := s.Store.GetLatestSubscriptionByUserConnector(ctx, paymentRow.UserID, paymentRow.ConnectorID); err != nil {
		slog.Error("load latest subscription failed", "error", err, "user_id", paymentRow.UserID, "connector_id", paymentRow.ConnectorID)
	} else if found && latestSub.Status == domain.SubscriptionStatusActive && latestSub.EndsAt.After(startAt) {
		startAt = latestSub.EndsAt
	}
	if connectorExists {
		endsAt = connector.SubscriptionEndsAt(startAt)
	} else {
		endsAt = startAt.AddDate(0, 0, 30)
	}
	if err := s.Store.UpsertSubscriptionByPayment(ctx, domain.Subscription{
		UserID:         paymentRow.UserID,
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

	if err := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
		ctx,
		paymentRow.UserID,
		"",
		paymentRow.ConnectorID,
		domain.AuditActionSubscriptionActivated,
		"payment_id="+strconv.FormatInt(paymentRow.ID, 10),
		now,
	)); err != nil {
		slog.Error("save audit event failed", "error", err, "action", domain.AuditActionSubscriptionActivated)
	}

	if !paymentMarkedNow {
		return
	}
	channelURL := ""
	if connectorExists {
		if s.BuildTelegramAccessLink != nil {
			accessLink, err := s.BuildTelegramAccessLink(ctx, paymentRow.UserID, connector)
			if err != nil {
				slog.Error("build telegram access link failed", "error", err, "user_id", paymentRow.UserID, "connector_id", connector.ID, "payment_id", paymentRow.ID)
			} else if strings.TrimSpace(accessLink) != "" {
				channelURL = accessLink
			}
		}
		if channelURL == "" {
			channelURL = s.ResolveConnectorChannel(connector.ChannelURL, connector.ChatID)
		}
	}
	successText := s.PaymentSuccessMessage(endsAt)
	message := messenger.OutgoingMessage{Text: successText}
	if channelURL != "" {
		message.Text += s.SuccessChannelHint
		message.Buttons = [][]messenger.ActionButton{
			{{Text: s.OpenChannelActionLabel, URL: channelURL}},
			{{Text: s.MySubscriptionAction, Action: "menu:subscription"}},
		}
	} else {
		message.Buttons = [][]messenger.ActionButton{
			{{Text: s.MySubscriptionAction, Action: "menu:subscription"}},
		}
	}
	if err := s.SendUserNotification(ctx, paymentRow.UserID, "", message); err != nil {
		slog.Error("send payment success message failed", "error", err, "user_id", paymentRow.UserID, "payment_id", paymentRow.ID)
		return
	}
	if err := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
		ctx,
		paymentRow.UserID,
		"",
		paymentRow.ConnectorID,
		domain.AuditActionPaymentSuccessNotified,
		"payment_id="+strconv.FormatInt(paymentRow.ID, 10),
		now,
	)); err != nil {
		slog.Error("save audit event failed", "error", err, "action", domain.AuditActionPaymentSuccessNotified)
	}
}

func (s *Service) NotifyFailedRecurringPayment(ctx context.Context, paymentRow domain.Payment) {
	if !paymentRow.AutoPayEnabled || paymentRow.SubscriptionID <= 0 {
		return
	}
	connector, found, err := s.Store.GetConnector(ctx, paymentRow.ConnectorID)
	if err != nil {
		slog.Error("load connector for failed recurring payment notification failed", "error", err, "connector_id", paymentRow.ConnectorID, "payment_id", paymentRow.ID)
		return
	}
	if !found {
		return
	}

	renewURL := buildBotStartURL(s.TelegramBotUsername, connector.StartPayload)
	message := messenger.OutgoingMessage{Text: s.FailedRecurringText}
	if renewURL != "" {
		message.Buttons = [][]messenger.ActionButton{{
			{Text: s.FailedRecurringButton, URL: renewURL},
		}}
	}

	if err := s.SendUserNotification(ctx, paymentRow.UserID, "", message); err != nil {
		slog.Warn("failed recurring payment notify failed", "payment_id", paymentRow.ID, "user_id", paymentRow.UserID, "error", err)
		return
	}
	if err := s.Store.SaveAuditEvent(ctx, s.BuildTargetAuditEvent(
		ctx,
		paymentRow.UserID,
		"",
		paymentRow.ConnectorID,
		domain.AuditActionRecurringPaymentFailedNotice,
		"payment_id="+strconv.FormatInt(paymentRow.ID, 10),
		time.Now().UTC(),
	)); err != nil {
		slog.Error("save audit event failed", "error", err, "action", domain.AuditActionRecurringPaymentFailedNotice)
	}
}

func buildBotStartURL(botUsername, startPayload string) string {
	username := strings.TrimSpace(strings.TrimPrefix(botUsername, "@"))
	payload := strings.TrimSpace(startPayload)
	if username == "" || payload == "" {
		return ""
	}
	return "https://t.me/" + username + "?start=" + payload
}
