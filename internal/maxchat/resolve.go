package maxchat

import (
	"net/url"
	"strconv"
	"strings"
)

// NormalizeChatID parses MAX chat identifiers stored in admin/config fields.
func NormalizeChatID(raw string) (int64, bool) {
	v := strings.TrimSpace(strings.TrimPrefix(raw, "+"))
	if v == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

// ResolveChatID prefers an explicit stored chat ID and falls back to parsing a
// numeric chat ID from a MAX public/web URL path such as:
// - https://max.ru/-72598909498032
// - https://web.max.ru/-72598909498032
func ResolveChatID(chatIDRaw, accessURL string) (int64, bool) {
	if id, ok := NormalizeChatID(chatIDRaw); ok {
		return id, true
	}
	return parseChatIDFromURL(accessURL)
}

func parseChatIDFromURL(raw string) (int64, bool) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0, false
	}
	parsed, err := url.Parse(v)
	if err != nil {
		return 0, false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	switch host {
	case "max.ru", "www.max.ru", "web.max.ru":
	default:
		return 0, false
	}
	path := strings.Trim(strings.TrimSpace(parsed.EscapedPath()), "/")
	if path == "" {
		path = strings.Trim(strings.TrimSpace(parsed.Path), "/")
	}
	if path == "" {
		return 0, false
	}
	segment := strings.Split(path, "/")[0]
	return NormalizeChatID(segment)
}
