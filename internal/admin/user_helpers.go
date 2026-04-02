package admin

import (
	"net/url"
	"strconv"
	"strings"
)

func buildUserDetailURL(lang string, userID int64) string {
	params := url.Values{}
	params.Set("lang", lang)
	if userID > 0 {
		params.Set("user_id", strconv.FormatInt(userID, 10))
	}
	return "/admin/users/view?" + params.Encode()
}

func buildSubscriptionRevokeURL(lang string, userID, subscriptionID int64) string {
	params := url.Values{}
	params.Set("lang", lang)
	if userID > 0 {
		params.Set("user_id", strconv.FormatInt(userID, 10))
	}
	params.Set("subscription_id", strconv.FormatInt(subscriptionID, 10))
	return "/admin/subscriptions/revoke?" + params.Encode()
}

func buildUserPaymentLinkURL(lang string, userID, subscriptionID int64) string {
	params := url.Values{}
	params.Set("lang", lang)
	if userID > 0 {
		params.Set("user_id", strconv.FormatInt(userID, 10))
	}
	params.Set("subscription_id", strconv.FormatInt(subscriptionID, 10))
	return "/admin/users/send-payment-link?" + params.Encode()
}

func buildSubscriptionRebillURL(lang string, userID, subscriptionID int64) string {
	params := url.Values{}
	params.Set("lang", lang)
	if userID > 0 {
		params.Set("user_id", strconv.FormatInt(userID, 10))
	}
	params.Set("subscription_id", strconv.FormatInt(subscriptionID, 10))
	return "/admin/subscriptions/rebill?" + params.Encode()
}

func parseInt64Default(raw string) int64 {
	value, _ := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	return value
}

func normalizeAdminTelegramChatID(chatIDRaw string) (int64, bool) {
	raw := strings.TrimSpace(chatIDRaw)
	if raw == "" {
		return 0, false
	}
	raw = strings.TrimPrefix(raw, "+")
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value == 0 {
		return 0, false
	}
	if value < 0 {
		return value, true
	}
	return -value, true
}

func buildAdminBotStartURL(botUsername, startPayload string) string {
	username := strings.TrimSpace(strings.TrimPrefix(botUsername, "@"))
	payload := strings.TrimSpace(startPayload)
	if username == "" || payload == "" {
		return ""
	}
	return "https://t.me/" + username + "?start=" + payload
}

func buildAdminMAXStartURL(botName, startPayload string) string {
	name := strings.TrimSpace(strings.TrimPrefix(botName, "@"))
	payload := strings.TrimSpace(startPayload)
	if name == "" || payload == "" {
		return ""
	}
	return "https://max.ru/" + name + "?start=" + payload
}

func buildAdminMAXChatURL(botName string) string {
	name := strings.TrimSpace(strings.TrimPrefix(botName, "@"))
	if name == "" {
		return ""
	}
	return "https://max.ru/" + name
}

func buildAdminStartCommand(startPayload string) string {
	payload := strings.TrimSpace(startPayload)
	if payload == "" {
		return ""
	}
	return "/start " + payload
}

func trimAuditDetails(raw string, limit int) string {
	text := strings.TrimSpace(raw)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	if limit <= 3 {
		return text[:limit]
	}
	return text[:limit-3] + "..."
}
