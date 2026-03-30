package bot

import (
	"context"
	"log/slog"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/channelurl"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

func (h *Handler) sendSubscriptionOverview(ctx context.Context, chatID int64, userIdentity messenger.UserIdentity) {
	subs, err := queryActiveSubscriptions(ctx, h, userIdentity)
	if err != nil {
		slog.Error("list active subscriptions failed", "error", err, "messenger_kind", userIdentity.Kind, "messenger_user_id", userIdentity.ID)
		h.sendTo(ctx, chatID, userIdentity, botMsgSubscriptionLoadFailed)
		return
	}

	text, ok := h.buildSubscriptionOverviewText(ctx, userIdentity, subs)
	if !ok {
		h.sendTo(ctx, chatID, userIdentity, botMsgNoActiveSubscriptions)
		return
	}

	h.sendTo(ctx, chatID, userIdentity, text)
	h.logAuditEvent(ctx, userIdentity, 0, domain.AuditActionMenuSubscriptionOpened, "")
}

func (h *Handler) buildSubscriptionOverviewText(ctx context.Context, userIdentity messenger.UserIdentity, subs []domain.Subscription) (string, bool) {
	if len(subs) == 0 {
		return "", false
	}

	lines := []string{botMenuSubscriptionHeader, ""}
	for _, sub := range subs {
		connector, ok, err := h.store.GetConnector(ctx, sub.ConnectorID)
		if err != nil {
			slog.Error("load connector for subscription overview failed", "error", err, "connector_id", sub.ConnectorID, "messenger_kind", userIdentity.Kind, "messenger_user_id", userIdentity.ID)
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

func (h *Handler) sendPaymentHistory(ctx context.Context, chatID int64, userIdentity messenger.UserIdentity) {
	user, resolved := h.resolveMessengerUser(ctx, userIdentity)
	if !resolved {
		h.sendTo(ctx, chatID, userIdentity, botMsgPaymentsLoadFailed)
		return
	}
	payments, err := h.store.ListPayments(ctx, domain.PaymentListQuery{
		UserID: user.ID,
		Limit:  5,
	})
	if err != nil {
		slog.Error("list payments for menu failed", "error", err, "messenger_kind", userIdentity.Kind, "messenger_user_id", userIdentity.ID)
		h.sendTo(ctx, chatID, userIdentity, botMsgPaymentsLoadFailed)
		return
	}

	text, ok := buildPaymentHistoryText(payments)
	if !ok {
		h.sendTo(ctx, chatID, userIdentity, botMsgNoPayments)
		return
	}

	h.sendTo(ctx, chatID, userIdentity, text)
	h.logAuditEvent(ctx, userIdentity, 0, domain.AuditActionMenuPaymentsOpened, "")
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
