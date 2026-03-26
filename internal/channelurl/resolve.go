package channelurl

import (
	"strconv"
	"strings"
)

// Resolve returns a public link for the connector destination.
// Full explicit URLs are preserved as-is so non-Telegram channels, including
// MAX web links, are not rewritten into Telegram form. Telegram shorthands such
// as `@name` or `t.me/name` are normalized for consistency.
func Resolve(channelURL, chatID string) string {
	explicit := strings.TrimSpace(channelURL)
	if explicit != "" {
		if isFullURL(explicit) {
			return explicit
		}
		if normalized := normalizeTelegramPublicLink(explicit); normalized != "" {
			return normalized
		}
	}
	return buildTelegramChatURL(chatID)
}

func isFullURL(raw string) bool {
	v := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "http://")
}

func buildTelegramChatURL(chatID string) string {
	raw := strings.TrimSpace(chatID)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "@") && len(raw) > 1 {
		return "https://t.me/" + strings.TrimPrefix(raw, "@")
	}
	if _, err := strconv.ParseInt(raw, 10, 64); err != nil {
		return "https://t.me/" + strings.TrimPrefix(raw, "@")
	}
	normalized := strings.TrimPrefix(raw, "-")
	normalized = strings.TrimPrefix(normalized, "100")
	if normalized == "" {
		return ""
	}
	return "https://t.me/c/" + normalized
}

func normalizeTelegramPublicLink(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	v = strings.TrimPrefix(v, "https://")
	v = strings.TrimPrefix(v, "http://")
	v = strings.TrimPrefix(v, "t.me/")
	v = strings.TrimPrefix(v, "telegram.me/")
	v = strings.TrimPrefix(v, "@")
	v = strings.TrimPrefix(v, "/")
	if v == "" || strings.Contains(v, " ") {
		return ""
	}
	return "https://t.me/" + v
}
