package bot

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"

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

	currentSubs, futureSubs := splitCurrentAndFutureSubscriptions(subs, time.Now().UTC())
	if len(currentSubs) == 0 && len(futureSubs) == 0 {
		return "", false
	}

	lines := []string{botMenuSubscriptionHeader, ""}
	if len(currentSubs) == 0 {
		lines = append(lines, botMenuSubscriptionNoCurrent, "")
	}
	for _, sub := range currentSubs {
		connector, ok, err := h.store.GetConnector(ctx, sub.ConnectorID)
		if err != nil {
			slog.Error("load connector for subscription overview failed", "error", err, "connector_id", sub.ConnectorID, "messenger_kind", userIdentity.Kind, "messenger_user_id", userIdentity.ID)
			continue
		}
		if !ok {
			continue
		}

		lines = append(lines, botSubscriptionOverviewLines(sub, connector, "")...)
		lines = append(lines, botSubscriptionAccessLines(connector, userIdentity.Kind)...)
		lines = append(lines, "")
	}
	if len(futureSubs) > 0 {
		lines = append(lines, botMenuSubscriptionNextHeader, "")
		for _, sub := range futureSubs {
			connector, ok, err := h.store.GetConnector(ctx, sub.ConnectorID)
			if err != nil {
				slog.Error("load connector for future subscription overview failed", "error", err, "connector_id", sub.ConnectorID, "messenger_kind", userIdentity.Kind, "messenger_user_id", userIdentity.ID)
				continue
			}
			if !ok {
				continue
			}
			lines = append(lines, botUpcomingSubscriptionLines(sub, connector)...)
			lines = append(lines, botSubscriptionAccessLines(connector, userIdentity.Kind)...)
			lines = append(lines, "")
		}
	}
	if len(lines) <= 2 || (len(currentSubs) == 0 && len(futureSubs) == 0) {
		return "", false
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), true
}

func splitCurrentAndFutureSubscriptions(subs []domain.Subscription, now time.Time) ([]domain.Subscription, []domain.Subscription) {
	current := make([]domain.Subscription, 0, len(subs))
	future := make([]domain.Subscription, 0, len(subs))
	for _, sub := range subs {
		if sub.Status != domain.SubscriptionStatusActive {
			continue
		}
		switch {
		case sub.StartsAt.After(now):
			future = append(future, sub)
		case sub.EndsAt.After(now):
			current = append(current, sub)
		}
	}
	sort.Slice(current, func(i, j int) bool {
		if current[i].EndsAt.Equal(current[j].EndsAt) {
			return current[i].ID < current[j].ID
		}
		return current[i].EndsAt.Before(current[j].EndsAt)
	})
	sort.Slice(future, func(i, j int) bool {
		if future[i].StartsAt.Equal(future[j].StartsAt) {
			return future[i].ID < future[j].ID
		}
		return future[i].StartsAt.Before(future[j].StartsAt)
	})
	return current, future
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
