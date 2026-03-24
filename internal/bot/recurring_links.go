package bot

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/recurringlink"
)

const autopayCancelTokenTTL = 180 * 24 * time.Hour

func (h *Handler) buildAutopayCancelURL(telegramID int64) string {
	baseURL := strings.TrimRight(strings.TrimSpace(h.publicBaseURL), "/")
	if baseURL == "" || strings.TrimSpace(h.encryptionKey) == "" || telegramID <= 0 {
		return ""
	}
	token, err := recurringlink.BuildCancelToken(h.encryptionKey, telegramID, time.Now().UTC().Add(autopayCancelTokenTTL))
	if err != nil {
		return ""
	}
	return baseURL + "/unsubscribe/" + url.PathEscape(token)
}

func (h *Handler) buildAutopayCheckoutURL(connectorID int64) string {
	baseURL := strings.TrimRight(strings.TrimSpace(h.publicBaseURL), "/")
	if baseURL == "" || connectorID <= 0 {
		return ""
	}
	return baseURL + "/subscribe/" + url.PathEscape(strconv.FormatInt(connectorID, 10))
}
