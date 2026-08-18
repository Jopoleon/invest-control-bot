package channelurl

import (
	"strconv"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/telegramlink"
)

// Resolve returns a public link for the connector destination.
// Supported Telegram access links are canonicalized to t.me. Telegram Web
// client URLs, private-message links and unknown HTTP(S) URLs are not valid
// access fallbacks and intentionally resolve to an empty user-facing URL.
func Resolve(channelURL, chatID string) string {
	explicit := strings.TrimSpace(channelURL)
	if explicit != "" {
		if destination, err := telegramlink.Parse(explicit); err == nil {
			switch destination.Kind {
			case telegramlink.KindPublic, telegramlink.KindInvite:
				return destination.CanonicalURL
			default:
				return ""
			}
		}
		return ""
	}
	return buildTelegramChatURL(chatID)
}

// ResolvePublic returns only a public username destination suitable for an
// unauthenticated page. Invite links are deliberately excluded so checkout or
// failed-payment pages cannot bypass paid access.
func ResolvePublic(channelURL, chatID string) string {
	candidate := Resolve(channelURL, chatID)
	if candidate == "" {
		return ""
	}
	destination, err := telegramlink.Parse(candidate)
	if err != nil || destination.Kind != telegramlink.KindPublic {
		return ""
	}
	return destination.CanonicalURL
}

func buildTelegramChatURL(chatID string) string {
	raw := strings.TrimSpace(chatID)
	if raw == "" {
		return ""
	}
	if _, err := strconv.ParseInt(strings.TrimPrefix(raw, "+"), 10, 64); err == nil {
		// Telegram has no official cross-client link for opening the root of a
		// private chat by Bot API ID alone. t.me/c additionally requires a
		// concrete message ID, so numeric IDs are used only for Bot API calls.
		return ""
	}
	destination, err := telegramlink.Parse(raw)
	if err != nil || destination.Kind != telegramlink.KindPublic {
		return ""
	}
	return destination.CanonicalURL
}
