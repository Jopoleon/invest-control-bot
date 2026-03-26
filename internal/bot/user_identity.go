package bot

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// resolveMessengerUser ensures there is one internal user linked to the current
// messenger identity and returns that user record for further bot logic.
func (h *Handler) resolveMessengerUser(ctx context.Context, identity messenger.UserIdentity) (domain.User, bool) {
	if identity.ID <= 0 {
		return domain.User{}, false
	}

	kind := domain.MessengerKindTelegram
	switch identity.Kind {
	case messenger.KindMAX:
		kind = domain.MessengerKindMAX
	}

	user, _, err := h.store.GetOrCreateUserByMessenger(
		ctx,
		kind,
		strconv.FormatInt(identity.ID, 10),
		identity.Username,
	)
	if err != nil {
		slog.Error("resolve messenger user failed", "error", err, "messenger_kind", identity.Kind, "external_user_id", identity.ID)
		return domain.User{}, false
	}
	return user, true
}
