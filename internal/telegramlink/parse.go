package telegramlink

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	canonicalHost       = "t.me"
	maxUsernameLength   = 32
	maxChannelID        = uint64(999_999_999_999)
	maxMessageID        = uint64(2_147_483_647)
	botAPIChannelPrefix = "-100"
)

var (
	// ErrInvalidDestination indicates that the input isn't one of the supported
	// Telegram destination forms.
	ErrInvalidDestination = errors.New("invalid Telegram destination")
	// ErrNoUserFacingURL indicates a valid import-only Telegram Web URL.
	ErrNoUserFacingURL = errors.New("Telegram destination has no user-facing URL")
	// ErrNoChatRef indicates a valid destination, such as an invite link, from
	// which a Bot API chat reference can't be derived.
	ErrNoChatRef = errors.New("Telegram destination has no chat reference")
)

// Kind identifies the supported Telegram destination form.
type Kind string

const (
	KindPublic         Kind = "public"
	KindInvite         Kind = "invite"
	KindPrivateMessage Kind = "private_message"
	KindWebImport      Kind = "web_import"
)

// Destination is a validated Telegram destination.
//
// CanonicalURL is intentionally empty for KindWebImport: web.telegram.org
// routes identify a browser-client screen and must never be sent to users as
// an application deep link. ChatRef is empty for invite links because an
// invite hash can't be reversed into a Bot API chat reference.
type Destination struct {
	Kind         Kind
	CanonicalURL string
	ChatRef      string
}

// IsImportOnly reports whether the input can only contribute a chat reference
// during connector import and has no safe user-facing destination URL.
func (d Destination) IsImportOnly() bool {
	return d.Kind == KindWebImport
}

// IsWebClientURL reports whether raw targets Telegram's browser client host.
// It intentionally ignores the route shape so malformed legacy WebA/K/Z URLs
// can still be blocked from user-facing buttons instead of being preserved as
// unknown HTTP links.
func IsWebClientURL(raw string) bool {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false
	}
	if !strings.Contains(value, "://") {
		if !strings.HasPrefix(strings.ToLower(value), "web.telegram.org/") {
			return false
		}
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	return host == "web.telegram.org"
}

// Parse validates raw and returns its canonical Telegram destination data.
func Parse(raw string) (Destination, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return Destination{}, invalid("destination is empty")
	}

	if strings.HasPrefix(value, "@") {
		return publicDestination(strings.TrimPrefix(value, "@"))
	}

	lower := strings.ToLower(value)
	switch {
	case strings.HasPrefix(lower, "t.me/"), strings.HasPrefix(lower, "telegram.me/"):
		return parseHTTPS("https://" + value)
	case strings.Contains(value, "://"):
		parsed, err := url.Parse(value)
		if err != nil {
			return Destination{}, invalid("malformed URL")
		}
		switch strings.ToLower(parsed.Scheme) {
		case "https":
			return parseParsedHTTPS(parsed)
		case "tg":
			return parseTG(parsed)
		default:
			return Destination{}, invalid("unsupported URL scheme")
		}
	default:
		return publicDestination(value)
	}
}

// Normalize returns the canonical user-facing URL and Bot API chat reference.
// Either value may be empty for a valid destination; Parse or Destination.Kind
// can be used when the caller needs to distinguish those cases.
func Normalize(raw string) (canonicalURL, chatRef string, err error) {
	destination, err := Parse(raw)
	if err != nil {
		return "", "", err
	}
	return destination.CanonicalURL, destination.ChatRef, nil
}

// UserFacingURL parses raw and returns its canonical URL. Import-only Telegram
// Web routes return ErrNoUserFacingURL.
func UserFacingURL(raw string) (string, error) {
	destination, err := Parse(raw)
	if err != nil {
		return "", err
	}
	if destination.CanonicalURL == "" {
		return "", ErrNoUserFacingURL
	}
	return destination.CanonicalURL, nil
}

// ResolveChatRef parses raw and returns a Bot API-compatible @username or
// -100... channel reference. Invite links return ErrNoChatRef.
func ResolveChatRef(raw string) (string, error) {
	destination, err := Parse(raw)
	if err != nil {
		return "", err
	}
	if destination.ChatRef == "" {
		return "", ErrNoChatRef
	}
	return destination.ChatRef, nil
}

func parseHTTPS(raw string) (Destination, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return Destination{}, invalid("malformed HTTPS URL")
	}
	return parseParsedHTTPS(parsed)
}

func parseParsedHTTPS(parsed *url.URL) (Destination, error) {
	if parsed == nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Opaque != "" || parsed.User != nil {
		return Destination{}, invalid("malformed HTTPS URL")
	}
	if parsed.Host == "" || strings.Contains(parsed.Host, ":") {
		return Destination{}, invalid("missing or non-canonical HTTPS host")
	}

	host := strings.ToLower(parsed.Host)
	switch host {
	case "t.me", "telegram.me":
		return parseTelegramHTTPS(parsed)
	case "web.telegram.org":
		return parseTelegramWebImport(parsed)
	default:
		return Destination{}, invalid("unsupported HTTPS host")
	}
}

func parseTelegramHTTPS(parsed *url.URL) (Destination, error) {
	if parsed.ForceQuery || parsed.Fragment != "" {
		return Destination{}, invalid("Telegram destination URL must not contain an empty query or fragment")
	}
	if parsed.RawQuery != "" && parsed.RawQuery != "single" {
		return Destination{}, invalid("unsupported Telegram destination query")
	}
	path, ok := strictPath(parsed)
	if !ok {
		return Destination{}, invalid("malformed Telegram destination path")
	}
	parts := splitPath(path)
	if len(parts) == 0 {
		return Destination{}, invalid("Telegram destination path is empty")
	}

	if len(parts) == 1 {
		if parsed.RawQuery != "" {
			return Destination{}, invalid("public and invite links must not contain a query")
		}
		if strings.HasPrefix(parts[0], "+") {
			hash := strings.TrimPrefix(parts[0], "+")
			return inviteDestination(hash)
		}
		if parts[0] == "c" || parts[0] == "joinchat" {
			return Destination{}, invalid("incomplete Telegram destination path")
		}
		return publicDestination(parts[0])
	}

	if len(parts) == 2 && parts[0] == "joinchat" {
		if parsed.RawQuery != "" {
			return Destination{}, invalid("invite links must not contain a query")
		}
		return inviteDestination(parts[1])
	}
	if len(parts) == 3 && parts[0] == "c" {
		return privateMessageDestination(parts[1], parts[2], parsed.RawQuery == "single")
	}
	return Destination{}, invalid("unsupported Telegram destination path")
}

func parseTelegramWebImport(parsed *url.URL) (Destination, error) {
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.User != nil {
		return Destination{}, invalid("malformed Telegram Web import URL")
	}
	path, ok := strictPath(parsed)
	if !ok || (parsed.Path != "/a/" && parsed.Path != "/k/" && parsed.Path != "/z/") || (path != "/a" && path != "/k" && path != "/z") {
		return Destination{}, invalid("unsupported Telegram Web client path")
	}
	if parsed.Fragment == "" || strings.Contains(parsed.RawFragment, "%") {
		return Destination{}, invalid("missing or encoded Telegram Web chat fragment")
	}
	channelID, ok := strings.CutPrefix(parsed.Fragment, botAPIChannelPrefix)
	if !ok || !validPositiveID(channelID, maxChannelID) {
		return Destination{}, invalid("malformed Telegram Web chat fragment")
	}
	return Destination{
		Kind:    KindWebImport,
		ChatRef: botAPIChannelPrefix + channelID,
	}, nil
}

func parseTG(parsed *url.URL) (Destination, error) {
	if parsed == nil || !strings.EqualFold(parsed.Scheme, "tg") || parsed.Opaque != "" || parsed.User != nil || parsed.Path != "" || parsed.Fragment != "" {
		return Destination{}, invalid("malformed tg URL")
	}
	if parsed.Host == "" || strings.Contains(parsed.Host, ":") {
		return Destination{}, invalid("malformed tg host")
	}

	switch strings.ToLower(parsed.Host) {
	case "resolve":
		domain, ok := strictSingleQueryValue(parsed, "domain")
		if !ok {
			return Destination{}, invalid("tg resolve requires only a domain parameter")
		}
		return publicDestination(domain)
	case "join":
		invite, ok := strictSingleQueryValue(parsed, "invite")
		if !ok {
			return Destination{}, invalid("tg join requires only an invite parameter")
		}
		return inviteDestination(invite)
	default:
		return Destination{}, invalid("unsupported tg destination")
	}
}

func strictSingleQueryValue(parsed *url.URL, key string) (string, bool) {
	prefix := key + "="
	if parsed == nil || parsed.ForceQuery || !strings.HasPrefix(parsed.RawQuery, prefix) || strings.ContainsAny(parsed.RawQuery, "&;%+") {
		return "", false
	}
	value := strings.TrimPrefix(parsed.RawQuery, prefix)
	if value == "" || strings.Contains(value, "=") {
		return "", false
	}
	return value, true
}

func publicDestination(username string) (Destination, error) {
	if !validUsername(username) {
		return Destination{}, invalid("malformed public username")
	}
	return Destination{
		Kind:         KindPublic,
		CanonicalURL: "https://" + canonicalHost + "/" + username,
		ChatRef:      "@" + username,
	}, nil
}

func inviteDestination(hash string) (Destination, error) {
	if !validInviteHash(hash) {
		return Destination{}, invalid("malformed invite hash")
	}
	return Destination{
		Kind:         KindInvite,
		CanonicalURL: "https://" + canonicalHost + "/+" + hash,
	}, nil
}

func privateMessageDestination(channelID, messageID string, single bool) (Destination, error) {
	if !validPositiveID(channelID, maxChannelID) || !validPositiveID(messageID, maxMessageID) {
		return Destination{}, invalid("private message link requires positive channel and message IDs")
	}
	canonicalURL := "https://" + canonicalHost + "/c/" + channelID + "/" + messageID
	if single {
		canonicalURL += "?single"
	}
	return Destination{
		Kind:         KindPrivateMessage,
		CanonicalURL: canonicalURL,
		ChatRef:      botAPIChannelPrefix + channelID,
	}, nil
}

func strictPath(parsed *url.URL) (string, bool) {
	if parsed == nil || parsed.Path == "" || strings.Contains(parsed.EscapedPath(), "%") || strings.Contains(parsed.Path, "//") {
		return "", false
	}
	path := parsed.Path
	if path != "/" {
		path = strings.TrimSuffix(path, "/")
	}
	if path == "" || !strings.HasPrefix(path, "/") {
		return "", false
	}
	return path, true
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func validUsername(value string) bool {
	if len(value) < 4 || len(value) > maxUsernameLength || value[0] < 'A' || (value[0] > 'Z' && value[0] < 'a') || value[0] > 'z' {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' {
			continue
		}
		return false
	}
	return true
}

func validInviteHash(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func validPositiveID(value string, max uint64) bool {
	if value == "" || value[0] < '1' || value[0] > '9' {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	numeric, err := strconv.ParseUint(value, 10, 64)
	return err == nil && numeric > 0 && numeric <= max
}

func invalid(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidDestination, reason)
}
