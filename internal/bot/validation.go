package bot

import (
	"regexp"
	"strings"
)

// e164PhoneRx validates international phone format used in onboarding.
var e164PhoneRx = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

// normalizePhone strips visual separators before validation/storage.
func normalizePhone(phone string) string {
	replacer := strings.NewReplacer(" ", "", "-", "", "(", "", ")", "")
	return replacer.Replace(strings.TrimSpace(phone))
}

// isValidE164 checks normalized phone against E.164 regex.
func isValidE164(phone string) bool {
	return e164PhoneRx.MatchString(phone)
}
