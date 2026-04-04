package domain

import (
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/channelurl"
)

// TelegramAccessURL resolves the Telegram destination configured for this
// connector using the backward-compatible telegram-only fields.
func (c Connector) TelegramAccessURL() string {
	return channelurl.Resolve(c.ChannelURL, c.ChatID)
}

// MAXAccessURL resolves the MAX destination configured for this connector.
func (c Connector) MAXAccessURL() string {
	return strings.TrimSpace(c.MAXChannelURL)
}

// AccessURL returns the access destination that matches the current messenger.
func (c Connector) AccessURL(kind MessengerKind) string {
	switch kind {
	case MessengerKindMAX:
		return c.MAXAccessURL()
	case MessengerKindTelegram:
		return c.TelegramAccessURL()
	default:
		return ""
	}
}

// HasAccessFor reports whether the connector can grant access in the selected messenger.
func (c Connector) HasAccessFor(kind MessengerKind) bool {
	return c.AccessURL(kind) != ""
}

// HasAnyAccessDestination reports whether at least one delivery destination is configured.
func (c Connector) HasAnyAccessDestination() bool {
	return c.TelegramAccessURL() != "" || c.MAXAccessURL() != ""
}
