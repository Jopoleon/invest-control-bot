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
