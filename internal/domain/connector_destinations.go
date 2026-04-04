package domain

import (
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/channelurl"
	"github.com/Jopoleon/invest-control-bot/internal/maxchat"
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

// ResolvedMAXChatID returns a MAX chat identifier suitable for membership
// operations. It prefers the explicit stored field and falls back to parsing a
// numeric chat ID from the configured MAX URL for backward compatibility.
func (c Connector) ResolvedMAXChatID() (int64, bool) {
	return maxchat.ResolveChatID(c.MAXChatID, c.MAXChannelURL)
}

// DeliveryMessengerKind resolves which messenger should be used for access
// delivery and membership operations for this connector. Single-destination
// connectors must not depend on a user's globally preferred account, while
// dual-destination connectors may still use the caller's fallback choice.
func (c Connector) DeliveryMessengerKind(fallback MessengerKind) MessengerKind {
	hasTelegram := c.HasAccessFor(MessengerKindTelegram)
	hasMAX := c.HasAccessFor(MessengerKindMAX)
	switch {
	case hasTelegram && !hasMAX:
		return MessengerKindTelegram
	case hasMAX && !hasTelegram:
		return MessengerKindMAX
	case hasTelegram && hasMAX:
		if fallback == MessengerKindMAX {
			return MessengerKindMAX
		}
		return MessengerKindTelegram
	default:
		return fallback
	}
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
