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
