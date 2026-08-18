package channelurl

import "testing"

func TestResolve_RejectsNonTelegramURL(t *testing.T) {
	for _, raw := range []string{
		"https://web.max.ru/-72598909498032",
		"https://web.telegram.org.evil.example/a/#-1003222018503",
		"https://example.com/channel",
	} {
		if got := Resolve(raw, ""); got != "" {
			t.Fatalf("Resolve(%q, empty) = %q, want empty", raw, got)
		}
	}
}

func TestResolve_NormalizesTelegramShorthand(t *testing.T) {
	got := Resolve("@test_channel", "")
	if got != "https://t.me/test_channel" {
		t.Fatalf("Resolve() = %q, want telegram public URL", got)
	}
}

func TestResolve_NormalizesSupportedTelegramAccessLinks(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "public username", raw: "t.me/test_channel", want: "https://t.me/test_channel"},
		{name: "telegram me public username", raw: "https://telegram.me/test_channel", want: "https://t.me/test_channel"},
		{name: "tg public username", raw: "tg://resolve?domain=test_channel", want: "https://t.me/test_channel"},
		{name: "canonical invite", raw: "https://t.me/+AbCd_123", want: "https://t.me/+AbCd_123"},
		{name: "legacy invite", raw: "https://telegram.me/joinchat/AbCd_123", want: "https://t.me/+AbCd_123"},
		{name: "tg invite", raw: "tg://join?invite=AbCd_123", want: "https://t.me/+AbCd_123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.raw, ""); got != tt.want {
				t.Fatalf("Resolve(%q, empty) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestResolve_DoesNotExposeTelegramWebClientURLs(t *testing.T) {
	for _, raw := range []string{
		"https://web.telegram.org/a/#-1003222018503",
		"https://web.telegram.org/k/#-1003222018503",
		"https://web.telegram.org/z/#-1003222018503",
		"http://web.telegram.org/a/#-1003222018503",
		"https://web.telegram.org:443/a/#-1003222018503",
		"https://web.telegram.org/broken",
	} {
		t.Run(raw, func(t *testing.T) {
			if got := Resolve(raw, ""); got != "" {
				t.Fatalf("Resolve(%q, empty) = %q, want empty user-facing URL", raw, got)
			}
		})
	}
}

func TestResolve_DoesNotBuildBarePrivateMessageLinkFromNumericChatID(t *testing.T) {
	for _, chatID := range []string{"1003626584986", "-1003626584986"} {
		t.Run(chatID, func(t *testing.T) {
			if got := Resolve("", chatID); got != "" {
				t.Fatalf("Resolve(empty, %q) = %q, want empty because t.me/c requires a message id", chatID, got)
			}
		})
	}
}

func TestResolve_DoesNotUsePrivateMessageLinkAsAccessFallback(t *testing.T) {
	if got := Resolve("https://t.me/c/3626584986/12", ""); got != "" {
		t.Fatalf("Resolve(private message, empty)=%q want empty access fallback", got)
	}
}

func TestResolvePublic_AllowsPublicUsernameButNotInvite(t *testing.T) {
	if got := ResolvePublic("https://telegram.me/public_channel", ""); got != "https://t.me/public_channel" {
		t.Fatalf("ResolvePublic(public)=%q want canonical public link", got)
	}
	if got := ResolvePublic("https://t.me/+AbCd_123", ""); got != "" {
		t.Fatalf("ResolvePublic(invite)=%q want empty", got)
	}
}
