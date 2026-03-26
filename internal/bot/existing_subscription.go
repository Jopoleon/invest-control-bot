package bot

import (
	"context"
	"fmt"
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

	text := fmt.Sprintf("У вас уже есть активная подписка «%s» до %s.", connector.Name, sub.EndsAt.In(time.Local).Format("02.01.2006 15:04"))
	var buttons [][]messenger.ActionButton

	if sub.AutoPayEnabled {
		text += "\n\nАвтоплатеж для этого тарифа уже включен."
		cancelURL := h.buildAutopayCancelURL(telegramID)
		if cancelURL != "" {
			buttons = [][]messenger.ActionButton{{
				buttonURL("Страница отключения", cancelURL),
			}}
		}
	} else {
		paymentRow, found, err := h.store.GetPaymentByID(ctx, sub.PaymentID)
		if err == nil && found && paymentRow.AutoPayEnabled {
			text += "\n\nАвтоплатеж для этого тарифа сейчас выключен, но его можно включить обратно без повторной оплаты."
			buttons = [][]messenger.ActionButton{{
				buttonAction("Включить автоплатеж обратно", menuCallbackAutopayOnSub+strconv.FormatInt(sub.ID, 10)),
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
