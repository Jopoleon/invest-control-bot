package telegramchat

import (
	"strconv"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/telegramlink"
)

// ResolveChatRef returns a Telegram Bot API chat reference from the explicit
// connector chat_id or, when it is omitted, from the public Telegram URL.
func ResolveChatRef(chatIDRaw, accessURL string) string {
	if ref := NormalizeChatRef(chatIDRaw); ref != "" {
		return ref
	}
	return parseChatRefFromURL(accessURL)
}

// NormalizeChatRef keeps explicit chat references in a Bot API-compatible form.
func NormalizeChatRef(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(value, "+")
	if numeric, err := strconv.ParseInt(value, 10, 64); err == nil && numeric != 0 {
		if numeric < 0 {
			return strconv.FormatInt(numeric, 10)
		}
		return "-" + strconv.FormatInt(numeric, 10)
	}
	destination, err := telegramlink.Parse(value)
	if err != nil {
		return ""
	}
	return destination.ChatRef
}

func parseChatRefFromURL(raw string) string {
	destination, err := telegramlink.Parse(raw)
	if err != nil {
		return ""
	}
	return destination.ChatRef
}
