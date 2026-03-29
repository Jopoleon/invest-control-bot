package bot

import (
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func normalizeMessengerUsername(raw string) string {
	return strings.TrimSpace(strings.TrimPrefix(raw, "@"))
}

func nextRegistrationStep(user domain.User, username string) domain.RegistrationStep {
	switch {
	case strings.TrimSpace(user.FullName) == "":
		return domain.StepFullName
	case strings.TrimSpace(user.Phone) == "":
		return domain.StepPhone
	case strings.TrimSpace(user.Email) == "":
		return domain.StepEmail
	case strings.TrimSpace(normalizeMessengerUsername(username)) == "":
		return domain.StepUsername
	default:
		return domain.StepDone
	}
}
