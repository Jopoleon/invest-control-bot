package channelurl

import "testing"

func TestResolve_PreservesExplicitMAXURL(t *testing.T) {
	got := Resolve("https://web.max.ru/-72598909498032", "")
	if got != "https://web.max.ru/-72598909498032" {
		t.Fatalf("Resolve() = %q, want explicit MAX URL", got)
	}
}

func TestResolve_NormalizesTelegramShorthand(t *testing.T) {
	got := Resolve("@test_channel", "")
	if got != "https://t.me/test_channel" {
		t.Fatalf("Resolve() = %q, want telegram public URL", got)
	}
}

func TestResolve_BuildsTelegramChatURLFromNumericChatID(t *testing.T) {
	got := Resolve("", "1003626584986")
	if got != "https://t.me/c/3626584986" {
		t.Fatalf("Resolve() = %q, want telegram chat URL", got)
	}
}
