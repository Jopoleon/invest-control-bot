package bot

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

func (h *Handler) sendExistingSubscriptionMessage(ctx context.Context, chatID, telegramID, connectorID int64) bool {
	sub, found, err := h.store.GetLatestSubscriptionByUserConnector(ctx, telegramID, connectorID)
	if err != nil {
		slog.Error("load latest subscription for connector failed", "error", err, "telegram_id", telegramID, "connector_id", connectorID)
		return false
	}
	if !found || sub.Status != domain.SubscriptionStatusActive || !sub.EndsAt.After(time.Now().UTC()) {
		return false
	}

	connector, found, err := h.store.GetConnector(ctx, connectorID)
	if err != nil || !found {
		return false
	}

	text := botExistingSubscriptionText(connector.Name, sub.EndsAt)
	var buttons [][]messenger.ActionButton

	if sub.AutoPayEnabled {
		text += botMsgExistingSubscriptionAutopayEnabled
		cancelURL := h.buildAutopayCancelURL(telegramID)
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
			return false
		}
	}

	if err := h.sender.Send(ctx, chatRef(chatID), messenger.OutgoingMessage{Text: text, Buttons: buttons}); err != nil {
		slog.Error("send existing subscription message failed", "error", err, "telegram_id", telegramID, "connector_id", connectorID)
		return false
	}
	return true
}
