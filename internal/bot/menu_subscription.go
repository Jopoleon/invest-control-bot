package bot

import (
	"context"
	"log/slog"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/channelurl"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

func (h *Handler) sendSubscriptionOverview(ctx context.Context, chatID, telegramID int64) {
	subs, err := queryActiveSubscriptions(ctx, h, telegramID)
	if err != nil {
		slog.Error("list active subscriptions failed", "error", err, "telegram_id", telegramID)
		h.send(ctx, chatID, botMsgSubscriptionLoadFailed)
		return
	}

	text, ok := h.buildSubscriptionOverviewText(ctx, telegramID, subs)
	if !ok {
		h.send(ctx, chatID, botMsgNoActiveSubscriptions)
		return
	}

	h.send(ctx, chatID, text)
	h.logAuditEvent(ctx, messenger.UserIdentity{Kind: messenger.KindTelegram, ID: telegramID}, 0, domain.AuditActionMenuSubscriptionOpened, "")
}

func (h *Handler) buildSubscriptionOverviewText(ctx context.Context, telegramID int64, subs []domain.Subscription) (string, bool) {
	if len(subs) == 0 {
		return "", false
	}

	lines := []string{botMenuSubscriptionHeader, ""}
	for _, sub := range subs {
		connector, ok, err := h.store.GetConnector(ctx, sub.ConnectorID)
		if err != nil {
			slog.Error("load connector for subscription overview failed", "error", err, "connector_id", sub.ConnectorID, "telegram_id", telegramID)
			continue
		}
		if !ok {
			continue
		}

		channel := resolveChannelForBot(connector.ChannelURL, connector.ChatID)
		lines = append(lines, botSubscriptionOverviewLines(sub, connector, channel)...)
	}
	if len(lines) <= 2 {
		return "", false
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), true
}

func (h *Handler) sendPaymentHistory(ctx context.Context, chatID, telegramID int64) {
	user, resolved := h.resolveMessengerUser(ctx, messenger.UserIdentity{Kind: messenger.KindTelegram, ID: telegramID})
	if !resolved {
		h.send(ctx, chatID, botMsgPaymentsLoadFailed)
		return
	}
	payments, err := h.store.ListPayments(ctx, domain.PaymentListQuery{
		UserID: user.ID,
		Limit:  5,
	})
	if err != nil {
		slog.Error("list payments for menu failed", "error", err, "telegram_id", telegramID)
		h.send(ctx, chatID, botMsgPaymentsLoadFailed)
		return
	}

	text, ok := buildPaymentHistoryText(payments)
	if !ok {
		h.send(ctx, chatID, botMsgNoPayments)
		return
	}

	h.send(ctx, chatID, text)
	h.logAuditEvent(ctx, messenger.UserIdentity{Kind: messenger.KindTelegram, ID: telegramID}, 0, domain.AuditActionMenuPaymentsOpened, "")
}

func buildPaymentHistoryText(payments []domain.Payment) (string, bool) {
	if len(payments) == 0 {
		return "", false
	}
	lines := []string{botMenuPaymentsHeader, ""}
	for _, p := range payments {
		lines = append(lines, botPaymentHistoryLines(p)...)
	}
	return strings.Join(lines, "\n"), true
}

func resolveChannelForBot(channelURL, chatID string) string {
	return channelurl.Resolve(channelURL, chatID)
}
