package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// sendViaMessengerAccount delivers admin-initiated messages through the
// selected linked messenger account. Telegram and MAX use different address
// fields (`chat_id` vs `user_id`), so this boundary keeps the transport detail
// out of individual admin actions.
func (h *Handler) sendViaMessengerAccount(ctx context.Context, account domain.UserMessengerAccount, msg messenger.OutgoingMessage) error {
	externalID, err := strconv.ParseInt(strings.TrimSpace(account.MessengerUserID), 10, 64)
	if err != nil || externalID <= 0 {
		return fmt.Errorf("invalid messenger user id %q for %s", account.MessengerUserID, account.MessengerKind)
	}

	switch account.MessengerKind {
	case domain.MessengerKindMAX:
		if h.maxSender == nil {
			return fmt.Errorf("max sender is not configured")
		}
		return h.maxSender.Send(ctx, messenger.UserRef{Kind: messenger.KindMAX, UserID: externalID}, msg)
	case domain.MessengerKindTelegram:
		if h.tg == nil {
			return fmt.Errorf("telegram sender is not configured")
		}
		return h.tg.Send(ctx, messenger.UserRef{Kind: messenger.KindTelegram, ChatID: externalID}, msg)
	default:
		return fmt.Errorf("unsupported messenger kind %q", account.MessengerKind)
	}
}

func (h *Handler) buildPaymentLinkMessage(lang string, account domain.UserMessengerAccount, connector domain.Connector) (messenger.OutgoingMessage, bool) {
	text := t(lang, "users.actions.paylink_text")
	if strings.TrimSpace(connector.Name) != "" {
		text = fmt.Sprintf(t(lang, "users.actions.paylink_text_named"), connector.Name)
	}

	var launchURL string
	switch account.MessengerKind {
	case domain.MessengerKindMAX:
		launchURL = buildAdminMAXStartURL(h.maxBotUsername, connector.StartPayload)
		if command := buildAdminStartCommand(connector.StartPayload); command != "" {
			text += "\n\n" + fmt.Sprintf(t(lang, "users.actions.paylink_max_fallback"), command)
		}
	default:
		launchURL = buildAdminBotStartURL(h.botUsername, connector.StartPayload)
	}

	msg := messenger.OutgoingMessage{Text: text}
	if launchURL != "" {
		msg.Buttons = [][]messenger.ActionButton{{
			{Text: t(lang, "users.actions.paylink_button"), URL: launchURL},
		}}
	}
	return msg, launchURL != "" || strings.TrimSpace(connector.StartPayload) != ""
}
