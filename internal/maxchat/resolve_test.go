package maxchat

import "testing"

func TestResolveChatID_PrefersExplicitChatID(t *testing.T) {
	t.Helper()

	got, ok := ResolveChatID("-72598909498032", "https://max.ru/123")
	if !ok || got != -72598909498032 {
		t.Fatalf("ResolveChatID explicit=(%d,%v) want (-72598909498032,true)", got, ok)
	}
}

func TestResolveChatID_ParsesNumericPathFromMAXURL(t *testing.T) {
	t.Helper()

	tests := []string{
		"https://max.ru/-72598909498032",
		"https://web.max.ru/-72598909498032",
	}
	for _, raw := range tests {
		got, ok := ResolveChatID("", raw)
		if !ok || got != -72598909498032 {
			t.Fatalf("ResolveChatID(%q)=(%d,%v) want (-72598909498032,true)", raw, got, ok)
		}
	}
}

func TestResolveChatID_RejectsNonNumericMAXURL(t *testing.T) {
	t.Helper()

	if _, ok := ResolveChatID("", "https://max.ru/test-connector"); ok {
		t.Fatal("ResolveChatID should reject non-numeric MAX public links")
	}
}

func TestNormalizeAccessURL_RewritesWebHostForUserFacingLinks(t *testing.T) {
	t.Helper()

	got := NormalizeAccessURL(" https://web.max.ru/-72598909498032?x=1 ")
	if got != "https://max.ru/-72598909498032?x=1" {
		t.Fatalf("NormalizeAccessURL()=%q want public max.ru URL", got)
	}
}
