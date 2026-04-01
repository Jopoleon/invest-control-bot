package admin

import (
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
)

type messengerAccountPresentation struct {
	DisplayName         string
	PrimaryAccount      string
	DirectMessageTarget string
	Accounts            []messengerAccountView
	HasTelegramIdentity bool
}

// buildMessengerAccountPresentation keeps admin templates transport-neutral by
// projecting raw linked accounts into consistent display strings.
func buildMessengerAccountPresentation(lang string, accounts []domain.UserMessengerAccount) messengerAccountPresentation {
	views := make([]messengerAccountView, 0, len(accounts))
	hasTelegramIdentity := false
	for _, account := range accounts {
		view := messengerAccountView{
			KindLabel:       messengerKindLabel(lang, account.MessengerKind),
			MessengerUserID: strings.TrimSpace(account.MessengerUserID),
			Username:        strings.TrimSpace(account.Username),
		}
		view.Display = buildMessengerAccountDisplay(view.KindLabel, view.MessengerUserID, view.Username)
		if account.MessengerKind == domain.MessengerKindTelegram && view.MessengerUserID != "" {
			hasTelegramIdentity = true
		}
		views = append(views, view)
	}

	primaryAccount := "—"
	directTarget := "—"
	displayName := ""
	if preferred, found := pickPreferredMessengerAccount(accounts); found {
		primaryLabel := buildMessengerAccountDisplay(
			messengerKindLabel(lang, preferred.MessengerKind),
			strings.TrimSpace(preferred.MessengerUserID),
			strings.TrimSpace(preferred.Username),
		)
		primaryAccount = primaryLabel
		directTarget = primaryLabel
		if username := strings.TrimSpace(preferred.Username); username != "" {
			displayName = username
		}
	}

	if displayName == "" {
		for _, view := range views {
			if view.Username != "" {
				displayName = view.Username
				break
			}
		}
	}

	return messengerAccountPresentation{
		DisplayName:         displayName,
		PrimaryAccount:      primaryAccount,
		DirectMessageTarget: directTarget,
		Accounts:            views,
		HasTelegramIdentity: hasTelegramIdentity,
	}
}

func buildMessengerAccountDisplay(kindLabel, messengerUserID, username string) string {
	parts := make([]string, 0, 3)
	if kindLabel != "" {
		parts = append(parts, kindLabel)
	}
	if messengerUserID != "" {
		parts = append(parts, messengerUserID)
	}
	if username != "" {
		parts = append(parts, "@"+strings.TrimPrefix(username, "@"))
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, " · ")
}

func messengerKindLabel(lang string, kind domain.MessengerKind) string {
	switch kind {
	case domain.MessengerKindMAX:
		return t(lang, "common.messenger.max")
	default:
		return t(lang, "common.messenger.telegram")
	}
}
