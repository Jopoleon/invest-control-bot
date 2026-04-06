package telegramchat

import (
	"net/url"
	"strconv"
	"strings"
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
	if strings.Contains(value, "://") || strings.HasPrefix(value, "t.me/") || strings.HasPrefix(value, "telegram.me/") {
		return parseChatRefFromURL(value)
	}
	if strings.HasPrefix(value, "@") {
		if len(value) == 1 {
			return ""
		}
		return value
	}
	value = strings.TrimPrefix(value, "+")
	if numeric, err := strconv.ParseInt(value, 10, 64); err == nil && numeric != 0 {
		if numeric < 0 {
			return strconv.FormatInt(numeric, 10)
		}
		return "-" + strconv.FormatInt(numeric, 10)
	}
	if strings.ContainsAny(value, " /") {
		return ""
	}
	return "@" + strings.TrimPrefix(value, "@")
}

func parseChatRefFromURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + strings.TrimPrefix(trimmed, "/")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimPrefix(parsed.Host, "www."))
	if host != "t.me" && host != "telegram.me" {
		return ""
	}
	path := strings.Trim(parsed.Path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	if parts[0] == "c" && len(parts) > 1 {
		if _, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
			return "-100" + parts[1]
		}
		return ""
	}
	if strings.HasPrefix(parts[0], "+") {
		return ""
	}
	return NormalizeChatRef(parts[0])
}
