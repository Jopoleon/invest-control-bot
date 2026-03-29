package admin

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
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
