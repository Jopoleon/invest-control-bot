package telegramlink

import (
	"errors"
	"testing"
)

func TestParseSupportedDestinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want Destination
	}{
		{
			name: "at username",
			raw:  "  @Invest_Channel  ",
			want: Destination{Kind: KindPublic, CanonicalURL: "https://t.me/Invest_Channel", ChatRef: "@Invest_Channel"},
		},
		{
			name: "bare username",
			raw:  "invest_channel",
			want: Destination{Kind: KindPublic, CanonicalURL: "https://t.me/invest_channel", ChatRef: "@invest_channel"},
		},
		{
			name: "scheme-less t.me",
			raw:  "t.me/invest_channel/",
			want: Destination{Kind: KindPublic, CanonicalURL: "https://t.me/invest_channel", ChatRef: "@invest_channel"},
		},
		{
			name: "telegram.me alias",
			raw:  "https://telegram.me/invest_channel",
			want: Destination{Kind: KindPublic, CanonicalURL: "https://t.me/invest_channel", ChatRef: "@invest_channel"},
		},
		{
			name: "modern invite",
			raw:  "https://t.me/+AbCd_123-xyz",
			want: Destination{Kind: KindInvite, CanonicalURL: "https://t.me/+AbCd_123-xyz"},
		},
		{
			name: "legacy invite",
			raw:  "telegram.me/joinchat/AbCd_123-xyz",
			want: Destination{Kind: KindInvite, CanonicalURL: "https://t.me/+AbCd_123-xyz"},
		},
		{
			name: "private message",
			raw:  "https://t.me/c/3626584986/12",
			want: Destination{Kind: KindPrivateMessage, CanonicalURL: "https://t.me/c/3626584986/12", ChatRef: "-1003626584986"},
		},
		{
			name: "private album message",
			raw:  "https://telegram.me/c/3626584986/12?single",
			want: Destination{Kind: KindPrivateMessage, CanonicalURL: "https://t.me/c/3626584986/12?single", ChatRef: "-1003626584986"},
		},
		{
			name: "tg public",
			raw:  "tg://resolve?domain=invest_channel",
			want: Destination{Kind: KindPublic, CanonicalURL: "https://t.me/invest_channel", ChatRef: "@invest_channel"},
		},
		{
			name: "tg invite",
			raw:  "tg://join?invite=AbCd_123-xyz",
			want: Destination{Kind: KindInvite, CanonicalURL: "https://t.me/+AbCd_123-xyz"},
		},
		{
			name: "Telegram Web A import",
			raw:  "https://web.telegram.org/a/#-1003626584986",
			want: Destination{Kind: KindWebImport, ChatRef: "-1003626584986"},
		},
		{
			name: "Telegram Web K import",
			raw:  "https://web.telegram.org/k/#-1001234567890",
			want: Destination{Kind: KindWebImport, ChatRef: "-1001234567890"},
		},
		{
			name: "Telegram Web Z import",
			raw:  "https://web.telegram.org/z/#-1009876543210",
			want: Destination{Kind: KindWebImport, ChatRef: "-1009876543210"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(tt.raw)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseRejectsUnsafeOrMalformedDestinations(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"@",
		"1234_channel",
		"bad-channel",
		"bad channel",
		"https://t.me",
		"http://t.me/invest_channel",
		"https://t.me.evil.example/invest_channel",
		"https://evil.example/t.me/invest_channel",
		"https://user@t.me/invest_channel",
		"https://t.me:443/invest_channel",
		"https://www.t.me/invest_channel",
		"https://t.me/invest_channel?profile=true",
		"https://t.me/invest_channel#fragment",
		"https://t.me/invest%5Fchannel",
		"https://t.me/+",
		"https://t.me/+bad.hash",
		"https://t.me/joinchat",
		"https://t.me/joinchat/hash/extra",
		"https://t.me/c/3626584986",
		"https://t.me/c/0/12",
		"https://t.me/c/03626584986/12",
		"https://t.me/c/3626584986/0",
		"https://t.me/c/3626584986/012",
		"https://t.me/c/3626584986/12?single=true",
		"https://t.me/c/3626584986/12?single&thread=1",
		"https://t.me/c/3626584986/12/extra",
		"https://web.telegram.org/a#-1003626584986",
		"https://web.telegram.org/a/#-100",
		"https://web.telegram.org/a/#-1000",
		"https://web.telegram.org/a/#-1003626584986/12",
		"https://web.telegram.org/b/#-1003626584986",
		"https://web.telegram.org/a/-1003626584986",
		"https://web.telegram.org/a/#3626584986",
		"https://web.telegram.org/a/?x=1#-1003626584986",
		"http://web.telegram.org/a/#-1003626584986",
		"https://web.telegram.org.evil.example/a/#-1003626584986",
		"tg://resolve",
		"tg://resolve?domain=@invest_channel",
		"tg://resolve?domain=invest_channel&post=1",
		"tg://resolve/path?domain=invest_channel",
		"tg://join?invite=bad.hash",
		"tg://join?invite=hash&extra=1",
		"tg://privatepost?channel=3626584986&post=12",
		"tg://user?id=12345",
		"ftp://t.me/invest_channel",
	}

	for _, raw := range tests {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(raw)
			if !errors.Is(err, ErrInvalidDestination) {
				t.Fatalf("Parse(%q) = %#v, %v; want ErrInvalidDestination", raw, got, err)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	canonicalURL, chatRef, err := Normalize("https://telegram.me/c/3626584986/12")
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if canonicalURL != "https://t.me/c/3626584986/12" || chatRef != "-1003626584986" {
		t.Fatalf("Normalize() = (%q, %q), want canonical private message destination", canonicalURL, chatRef)
	}

	canonicalURL, chatRef, err = Normalize("https://web.telegram.org/a/#-1003626584986")
	if err != nil {
		t.Fatalf("Normalize(import) error = %v", err)
	}
	if canonicalURL != "" || chatRef != "-1003626584986" {
		t.Fatalf("Normalize(import) = (%q, %q), want empty URL and import chat ref", canonicalURL, chatRef)
	}
}

func TestUserFacingURLRejectsImportOnlyURL(t *testing.T) {
	t.Parallel()

	got, err := UserFacingURL("https://web.telegram.org/a/#-1003626584986")
	if got != "" || !errors.Is(err, ErrNoUserFacingURL) {
		t.Fatalf("UserFacingURL() = %q, %v; want ErrNoUserFacingURL", got, err)
	}
}

func TestResolveChatRef(t *testing.T) {
	t.Parallel()

	got, err := ResolveChatRef("tg://resolve?domain=invest_channel")
	if err != nil || got != "@invest_channel" {
		t.Fatalf("ResolveChatRef(public) = %q, %v; want @invest_channel", got, err)
	}

	got, err = ResolveChatRef("https://t.me/+AbCd_123-xyz")
	if got != "" || !errors.Is(err, ErrNoChatRef) {
		t.Fatalf("ResolveChatRef(invite) = %q, %v; want ErrNoChatRef", got, err)
	}
}

func TestDestinationIsImportOnly(t *testing.T) {
	t.Parallel()

	web, err := Parse("https://web.telegram.org/a/#-1003626584986")
	if err != nil || !web.IsImportOnly() {
		t.Fatalf("web destination = %#v, %v; want import-only", web, err)
	}
	public, err := Parse("@invest_channel")
	if err != nil || public.IsImportOnly() {
		t.Fatalf("public destination = %#v, %v; want user-facing", public, err)
	}
}

func TestIsWebClientURL(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"https://web.telegram.org/a/#-1003626584986",
		"http://web.telegram.org/broken",
		"web.telegram.org/k/#-1003626584986",
		"https://user@web.telegram.org/a/#-1003626584986",
		"https://web.telegram.org:443/a/#-1003626584986",
		"https://web.telegram.org./a/#-1003626584986",
	} {
		if !IsWebClientURL(raw) {
			t.Errorf("IsWebClientURL(%q)=false want true", raw)
		}
	}
	for _, raw := range []string{
		"https://web.telegram.org.evil.example/a/#-1003626584986",
		"https://t.me/public_channel",
	} {
		if IsWebClientURL(raw) {
			t.Errorf("IsWebClientURL(%q)=true want false", raw)
		}
	}
}
