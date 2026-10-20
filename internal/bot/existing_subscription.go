package bot

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/app/periodpolicy"
	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// sendExistingSubscriptionMessage stops checkout when the user already has an
// active subscription for the connector and tells them about it.
//
// It returns handled=true when the message was sent and checkout must stop.
// When the active subscription relies on autopay but its rebill is stale (see
// periodpolicy.IsPendingRebillStale), it returns handled=false with a notice:
// the user may renew manually, and the paid period is queued after the current
// one by the usual max(now, current period end) rule.
func (h *Handler) sendExistingSubscriptionMessage(ctx context.Context, chatID int64, userIdentity messenger.UserIdentity, connectorID int64) (handled bool, manualRenewalNotice string) {
	user, resolved := h.resolveMessengerUser(ctx, userIdentity)
	if !resolved {
		return false, ""
	}

	now := time.Now().UTC()
	sub, found, err := h.store.GetLatestSubscriptionByUserConnector(ctx, user.ID, connectorID)
	if err != nil {
		slog.Error("load latest subscription for connector failed", "error", err, "user_id", user.ID, "connector_id", connectorID)
		return false, ""
	}
	if !found || sub.Status != domain.SubscriptionStatusActive || !sub.EndsAt.After(now) {
		return false, ""
	}

	connector, found, err := h.store.GetConnector(ctx, connectorID)
	if err != nil || !found {
		return false, ""
	}

	text := botExistingSubscriptionText(connector.Name, sub.EndsAt)
	var buttons [][]messenger.ActionButton

	if sub.AutoPayEnabled {
		if h.hasStalePendingRebill(ctx, sub, connector, now) {
			return false, text + botMsgExistingSubscriptionRebillNotConfirmed
		}
		text += botMsgExistingSubscriptionAutopayEnabled
		cancelURL := h.buildAutopayCancelURL(userIdentity.ID)
		if cancelURL != "" {
			buttons = [][]messenger.ActionButton{{
				buttonURL(botBtnAutopayCancelPage, cancelURL),
			}}
		}
	} else {
		paymentRow, found, err := h.store.GetPaymentByID(ctx, sub.PaymentID)
		if err == nil && found && paymentRow.AutoPayEnabled {
			text += botMsgExistingSubscriptionAutopayDisabledHint
			buttons = [][]messenger.ActionButton{{
				buttonAction(botBtnAutopayEnableAgain, menuCallbackAutopayOnSub+strconv.FormatInt(sub.ID, 10)),
			}}
		} else {
			return false, ""
		}
	}

	if err := h.sender.Send(ctx, recipientRef(chatID, userIdentity), messenger.OutgoingMessage{Text: text, Buttons: buttons}); err != nil {
		slog.Error("send existing subscription message failed", "error", err, "messenger_kind", userIdentity.Kind, "messenger_user_id", userIdentity.ID, "connector_id", connectorID)
		return false, ""
	}
	return true, ""
}

// hasStalePendingRebill reports whether the subscription has a pending rebill
// that never received the provider result callback in time. Robokassa sends no
// callback for a declined charge, so such a rebill would otherwise block the
// user from paying until access is revoked.
//
// TODO: If a stale rebill still succeeds after the user renewed manually, both
// payments activate a period. This is rare (production successes arrive within
// seconds) but needs operator follow-up when it happens.
func (h *Handler) hasStalePendingRebill(ctx context.Context, sub domain.Subscription, connector domain.Connector, now time.Time) bool {
	pending, found, err := h.store.GetPendingRebillPaymentBySubscription(ctx, sub.ID)
	if err != nil {
		slog.Error("load pending rebill for existing subscription failed", "error", err, "subscription_id", sub.ID)
		return false
	}
	if !found || pending.Status != domain.PaymentStatusPending {
		return false
	}
	return periodpolicy.Resolve(connector).IsPendingRebillStale(now, pending.CreatedAt, sub.EndsAt)
}
