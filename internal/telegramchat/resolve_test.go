package telegramchat

import "testing"

func TestResolveChatRef_PrefersExplicitNumericChatID(t *testing.T) {
	if got := ResolveChatRef("1003626584986", "https://t.me/testtestinvest"); got != "-1003626584986" {
		t.Fatalf("ResolveChatRef() = %q, want -1003626584986", got)
	}
}

func TestResolveChatRef_UsesUsernameFromPublicURL(t *testing.T) {
	if got := ResolveChatRef("", "https://t.me/testtestinvest"); got != "@testtestinvest" {
		t.Fatalf("ResolveChatRef() = %q, want @testtestinvest", got)
	}
}

func TestResolveChatRef_ImportsNumericChatIDFromTelegramWebURL(t *testing.T) {
	for _, raw := range []string{
		"https://web.telegram.org/a/#-1003222018503",
		"https://web.telegram.org/k/#-1003222018503",
		"https://web.telegram.org/z/#-1003222018503",
	} {
		t.Run(raw, func(t *testing.T) {
			if got := ResolveChatRef("", raw); got != "-1003222018503" {
				t.Fatalf("ResolveChatRef(empty, %q) = %q, want -1003222018503", raw, got)
			}
		})
	}
}

func TestResolveChatRef_RejectsBarePrivateMessageLink(t *testing.T) {
	if got := ResolveChatRef("", "https://t.me/c/3626584986"); got != "" {
		t.Fatalf("ResolveChatRef() = %q, want empty for t.me/c link without message id", got)
	}
}

func TestResolveChatRef_UsesNumericPathFromValidPrivateMessageLink(t *testing.T) {
	for _, raw := range []string{
		"https://t.me/c/3626584986/12",
		"https://t.me/c/3626584986/12?single",
	} {
		t.Run(raw, func(t *testing.T) {
			if got := ResolveChatRef("", raw); got != "-1003626584986" {
				t.Fatalf("ResolveChatRef(empty, %q) = %q, want -1003626584986", raw, got)
			}
		})
	}
}

func TestResolveChatRef_RejectsInviteLinks(t *testing.T) {
	if got := ResolveChatRef("", "https://t.me/+abcdef"); got != "" {
		t.Fatalf("ResolveChatRef() = %q, want empty", got)
	}
}
