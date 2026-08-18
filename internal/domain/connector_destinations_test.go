package domain

import "testing"

func TestConnectorDeliveryMessengerKind(t *testing.T) {
	tests := []struct {
		name      string
		connector Connector
		fallback  MessengerKind
		want      MessengerKind
	}{
		{
			name: "telegram only ignores max fallback",
			connector: Connector{
				ChatID: "1001234567890",
			},
			fallback: MessengerKindMAX,
			want:     MessengerKindTelegram,
		},
		{
			name: "max only ignores telegram fallback",
			connector: Connector{
				MAXChatID:     "-72598909498032",
				MAXChannelURL: "https://web.max.ru/-72598909498032",
			},
			fallback: MessengerKindTelegram,
			want:     MessengerKindMAX,
		},
		{
			name: "dual destination respects max fallback",
			connector: Connector{
				ChatID:        "1001234567890",
				MAXChatID:     "-72598909498032",
				MAXChannelURL: "https://web.max.ru/-72598909498032",
			},
			fallback: MessengerKindMAX,
			want:     MessengerKindMAX,
		},
		{
			name: "dual destination defaults to telegram",
			connector: Connector{
				ChatID:        "1001234567890",
				MAXChatID:     "-72598909498032",
				MAXChannelURL: "https://web.max.ru/-72598909498032",
			},
			fallback: MessengerKindTelegram,
			want:     MessengerKindTelegram,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.connector.DeliveryMessengerKind(tt.fallback); got != tt.want {
				t.Fatalf("DeliveryMessengerKind()=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestConnectorResolvedTelegramChatRef(t *testing.T) {
	tests := []struct {
		name      string
		connector Connector
		want      string
	}{
		{
			name: "prefers explicit numeric chat id",
			connector: Connector{
				ChatID:     "1001234567890",
				ChannelURL: "https://t.me/testtestinvest",
			},
			want: "-1001234567890",
		},
		{
			name: "falls back to public telegram url",
			connector: Connector{
				ChannelURL: "https://t.me/testtestinvest",
			},
			want: "@testtestinvest",
		},
		{
			name: "parses internal c link when only url is set",
			connector: Connector{
				ChannelURL: "https://t.me/c/3626584986/12",
			},
			want: "-1003626584986",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.connector.ResolvedTelegramChatRef(); got != tt.want {
				t.Fatalf("ResolvedTelegramChatRef()=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestConnectorBoundPrivateTelegramChatCountsAsAccessWithoutPublicURL(t *testing.T) {
	connector := Connector{ChatID: "1003222018503"}

	if got := connector.TelegramAccessURL(); got != "" {
		t.Fatalf("TelegramAccessURL()=%q want empty for a private numeric chat without a public link", got)
	}
	if got := connector.ResolvedTelegramChatRef(); got != "-1003222018503" {
		t.Fatalf("ResolvedTelegramChatRef()=%q want -1003222018503", got)
	}
	if !connector.HasAccessFor(MessengerKindTelegram) {
		t.Fatal("HasAccessFor(telegram)=false want true for a bound private chat")
	}
	if !connector.HasAnyAccessDestination() {
		t.Fatal("HasAnyAccessDestination()=false want true for a bound private chat")
	}
}

func TestConnectorTelegramWebURLIsImportHintButNeverUserFacing(t *testing.T) {
	connector := Connector{ChannelURL: "https://web.telegram.org/a/#-1003222018503"}

	if got := connector.TelegramAccessURL(); got != "" {
		t.Fatalf("TelegramAccessURL()=%q want empty for Telegram web-client URL", got)
	}
	if got := connector.AccessURL(MessengerKindTelegram); got != "" {
		t.Fatalf("AccessURL(telegram)=%q want empty for Telegram web-client URL", got)
	}
	if got := connector.ResolvedTelegramChatRef(); got != "-1003222018503" {
		t.Fatalf("ResolvedTelegramChatRef()=%q want imported -1003222018503", got)
	}
	if !connector.HasAccessFor(MessengerKindTelegram) {
		t.Fatal("HasAccessFor(telegram)=false want true when Telegram web URL supplies an importable chat id")
	}
}

func TestConnectorPublicAccessURL_AllowsPublicTelegramButNotPrivateLinks(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "public username", raw: "https://telegram.me/public_channel", want: "https://t.me/public_channel"},
		{name: "static invite", raw: "https://t.me/+AbCd_123"},
		{name: "web client import", raw: "https://web.telegram.org/a/#-1003222018503"},
		{name: "private message", raw: "https://t.me/c/3222018503/12"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connector := Connector{ChannelURL: tt.raw}
			if got := connector.PublicAccessURL(MessengerKindTelegram); got != tt.want {
				t.Fatalf("PublicAccessURL(telegram)=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestConnectorMAXAccessURL_NormalizesWebHostForUserFacingLinks(t *testing.T) {
	connector := Connector{MAXChannelURL: " https://web.max.ru/-72598909498032 "}
	if got := connector.MAXAccessURL(); got != "https://max.ru/-72598909498032" {
		t.Fatalf("MAXAccessURL()=%q want https://max.ru/-72598909498032", got)
	}
}

func TestConnectorMAXAccessURL_PreservesMaxHostAndPath(t *testing.T) {
	connector := Connector{MAXChannelURL: "https://max.ru/-72598909498032?x=1"}
	if got := connector.MAXAccessURL(); got != "https://max.ru/-72598909498032?x=1" {
		t.Fatalf("MAXAccessURL()=%q want original max.ru url", got)
	}
}
