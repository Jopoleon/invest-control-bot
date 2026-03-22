package bot

import (
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

func normalizeTelegramUsername(raw string) string {
	return strings.TrimSpace(strings.TrimPrefix(raw, "@"))
}

func applyCurrentTelegramUsername(user *domain.User, username string) bool {
	if user == nil {
		return false
	}
	normalized := normalizeTelegramUsername(username)
	if normalized == "" || user.TelegramUsername == normalized {
		return false
	}
	user.TelegramUsername = normalized
	return true
}

func nextRegistrationStep(user domain.User) domain.RegistrationStep {
	switch {
	case strings.TrimSpace(user.FullName) == "":
		return domain.StepFullName
	case strings.TrimSpace(user.Phone) == "":
		return domain.StepPhone
	case strings.TrimSpace(user.Email) == "":
		return domain.StepEmail
	case strings.TrimSpace(user.TelegramUsername) == "":
		return domain.StepUsername
	default:
		return domain.StepDone
	}
}

func registrationPrompt(step domain.RegistrationStep) string {
	switch step {
	case domain.StepFullName:
		return "ФИО"
	case domain.StepPhone:
		return "Телефон"
	case domain.StepEmail:
		return "E-mail"
	case domain.StepUsername:
		return "Ник телеграм"
	default:
		return ""
	}
}
