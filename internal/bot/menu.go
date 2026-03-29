package bot

import (
	"context"
	"log/slog"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

const (
	menuCallbackPrefix        = "menu:"
	menuCallbackSubscription  = "menu:subscription"
	menuCallbackPayments      = "menu:payments"
	menuCallbackAutopay       = "menu:autopay"
	menuCallbackAutopayPick   = "menu:autopay:pick"
	menuCallbackAutopayOn     = "menu:autopay:on"
	menuCallbackAutopayOnSub  = "menu:autopay:on:sub:"
	menuCallbackAutopayOffSub = "menu:autopay:off:sub:"
	menuCallbackAutopayOff    = "menu:autopay:off"
	menuCallbackAutopayOffAsk = "menu:autopay:off:ask"
	menuCallbackAutopayOffNo  = "menu:autopay:off:no"
)

func (h *Handler) sendMenu(ctx context.Context, chatID int64) {
	rows := [][]messenger.ActionButton{
		{
			buttonAction(botMenuButtonSubscription, menuCallbackSubscription),
		},
		{
			buttonAction(botMenuButtonPayments, menuCallbackPayments),
		},
	}
	if h.recurringEnabled {
		rows = append(rows, []messenger.ActionButton{
			buttonAction(botMenuButtonAutopay, menuCallbackAutopay),
		})
	}
	if err := h.sender.Send(ctx, chatRef(chatID), messenger.OutgoingMessage{Text: botMenuTitle, Buttons: rows}); err != nil {
		slog.Error("send menu failed", "error", err, "chat_id", chatID)
	}
}

// handleMenuCallback keeps callback routing in one place while detailed menu
// flows live in smaller files. This preserves a single callback dispatch table
// without forcing all subscription/payment/autopay behavior into one source file.
func (h *Handler) handleMenuCallback(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 {
		return
	}
	chatID := cb.ChatID
	if strings.HasPrefix(cb.Data, menuCallbackAutopayOnSub) {
		h.reactivateAutopayForSubscription(ctx, cb)
		return
	}
	if strings.HasPrefix(cb.Data, menuCallbackAutopayOffSub) {
		h.disableAutopayForSubscription(ctx, cb)
		return
	}

	handleMenuSelection(ctx, h, chatID, cb)
}

func handleMenuSelection(ctx context.Context, h *Handler, chatID int64, cb messenger.IncomingAction) {
	switch cb.Data {
	case menuCallbackSubscription:
		h.sendSubscriptionOverview(ctx, chatID, cb.User.ID)
	case menuCallbackPayments:
		h.sendPaymentHistory(ctx, chatID, cb.User.ID)
	case menuCallbackAutopay:
		h.sendAutopayInfo(ctx, chatID, cb.User.ID)
	case menuCallbackAutopayPick:
		h.showAutopaySubscriptionChooser(ctx, cb)
	case menuCallbackAutopayOn:
		h.setAutopayPreference(ctx, chatID, cb.User.ID, true)
	case menuCallbackAutopayOffAsk:
		h.confirmAutopayDisable(ctx, cb)
	case menuCallbackAutopayOff:
		h.disableAutopayConfirmed(ctx, cb)
	case menuCallbackAutopayOffNo:
		h.restoreAutopayInfo(ctx, cb)
	default:
		h.send(ctx, chatID, botMsgUnknownMenuCommand)
	}
}

func queryActiveSubscriptions(ctx context.Context, h *Handler, telegramID int64) ([]domain.Subscription, error) {
	user, resolved := h.resolveMessengerUser(ctx, messenger.UserIdentity{Kind: messenger.KindTelegram, ID: telegramID})
	if !resolved {
		return nil, nil
	}
	return h.store.ListSubscriptions(ctx, domain.SubscriptionListQuery{
		UserID: user.ID,
		Status: domain.SubscriptionStatusActive,
		Limit:  20,
	})
}
