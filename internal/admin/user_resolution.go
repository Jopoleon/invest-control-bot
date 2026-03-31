package admin

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// resolveUser prefers the internal user ID and falls back to the legacy Telegram-based lookup.
// This keeps existing admin URLs and forms working while detail pages gradually move to user_id.
func (h *Handler) resolveUser(ctx context.Context, userID, telegramID int64) (domain.User, bool, error) {
	if userID > 0 {
		return h.store.GetUserByID(ctx, userID)
	}
	if telegramID <= 0 {
		return domain.User{}, false, nil
	}
	return h.store.GetUserByMessenger(ctx, domain.MessengerKindTelegram, strconv.FormatInt(telegramID, 10))
}

func (h *Handler) resolveTelegramIdentity(ctx context.Context, userID int64) (int64, string, bool, error) {
	if userID <= 0 {
		return 0, "", false, nil
	}
	accounts, err := h.store.ListUserMessengerAccounts(ctx, userID)
	if err != nil {
		return 0, "", false, err
	}
	for _, account := range accounts {
		if account.MessengerKind != domain.MessengerKindTelegram {
			continue
		}
		telegramID, err := strconv.ParseInt(strings.TrimSpace(account.MessengerUserID), 10, 64)
		if err != nil || telegramID <= 0 {
			return 0, account.Username, false, nil
		}
		return telegramID, account.Username, true, nil
	}
	return 0, "", false, nil
}

func (h *Handler) buildTelegramIdentityLookup(ctx context.Context) func(userID int64) (int64, string, bool, error) {
	type cachedIdentity struct {
		telegramID int64
		username   string
		found      bool
		err        error
	}
	cache := make(map[int64]cachedIdentity)
	return func(userID int64) (int64, string, bool, error) {
		if cached, ok := cache[userID]; ok {
			return cached.telegramID, cached.username, cached.found, cached.err
		}
		telegramID, username, found, err := h.resolveTelegramIdentity(ctx, userID)
		cache[userID] = cachedIdentity{
			telegramID: telegramID,
			username:   username,
			found:      found,
			err:        err,
		}
		return telegramID, username, found, err
	}
}

func (h *Handler) resolvePreferredMessengerAccount(ctx context.Context, userID int64) (domain.UserMessengerAccount, bool, error) {
	if userID <= 0 {
		return domain.UserMessengerAccount{}, false, nil
	}
	accounts, err := h.store.ListUserMessengerAccounts(ctx, userID)
	if err != nil {
		return domain.UserMessengerAccount{}, false, err
	}
	account, found := pickPreferredMessengerAccount(accounts)
	return account, found, nil
}

func pickPreferredMessengerAccount(accounts []domain.UserMessengerAccount) (domain.UserMessengerAccount, bool) {
	for _, account := range accounts {
		if account.MessengerKind == domain.MessengerKindTelegram {
			return account, true
		}
	}
	for _, account := range accounts {
		if account.MessengerKind == domain.MessengerKindMAX {
			return account, true
		}
	}
	if len(accounts) == 0 {
		return domain.UserMessengerAccount{}, false
	}
	return accounts[0], true
}

func senderKindForAccount(account domain.UserMessengerAccount) messenger.Kind {
	switch account.MessengerKind {
	case domain.MessengerKindMAX:
		return messenger.KindMAX
	default:
		return messenger.KindTelegram
	}
}

func parseUserDetailParams(rawUserID, rawTelegramID string) (int64, int64) {
	return parseInt64Default(strings.TrimSpace(rawUserID)), parseInt64Default(strings.TrimSpace(rawTelegramID))
}

func (h *Handler) renderUserDetailForIDs(ctx context.Context, w http.ResponseWriter, r *http.Request, lang string, userID, telegramID int64, notice string) {
	user, found, err := h.resolveUser(ctx, userID, telegramID)
	if err != nil {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.load_error"))
		return
	}
	if !found {
		renderUserDetailError(h, w, r, lang, t(lang, "users.detail.not_found"))
		return
	}
	h.renderResolvedUserDetailPage(ctx, w, r, lang, user, notice)
}

// resolveFilterTelegramID keeps current list/report queries compatible with the
// legacy telegram_id-based store API while admin routes gradually accept user_id.
func (h *Handler) resolveFilterTelegramID(ctx context.Context, rawUserID, rawTelegramID string) (int64, error) {
	userID, telegramID := parseUserDetailParams(rawUserID, rawTelegramID)
	if userID <= 0 {
		return telegramID, nil
	}
	_, found, err := h.resolveUser(ctx, userID, 0)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, nil
	}
	resolvedTelegramID, _, found, err := h.resolveTelegramIdentity(ctx, userID)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, nil
	}
	return resolvedTelegramID, nil
}

// resolveFilterUserID lets admin list/report pages accept either internal user_id
// or the legacy telegram_id filter while query contracts move to user_id-first.
func (h *Handler) resolveFilterUserID(ctx context.Context, rawUserID, rawTelegramID string) (int64, error) {
	userID, telegramID := parseUserDetailParams(rawUserID, rawTelegramID)
	if userID > 0 {
		_, found, err := h.resolveUser(ctx, userID, 0)
		if err != nil {
			return 0, err
		}
		if !found {
			return 0, nil
		}
		return userID, nil
	}
	if telegramID <= 0 {
		return 0, nil
	}
	user, found, err := h.resolveUser(ctx, 0, telegramID)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, nil
	}
	return user.ID, nil
}
