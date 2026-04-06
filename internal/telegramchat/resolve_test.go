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

func TestResolveChatRef_UsesNumericPathFromInternalURL(t *testing.T) {
	if got := ResolveChatRef("", "https://t.me/c/3626584986/12"); got != "-1003626584986" {
		t.Fatalf("ResolveChatRef() = %q, want -1003626584986", got)
	}
}

func TestResolveChatRef_RejectsInviteLinks(t *testing.T) {
	if got := ResolveChatRef("", "https://t.me/+abcdef"); got != "" {
		t.Fatalf("ResolveChatRef() = %q, want empty", got)
	}
}
