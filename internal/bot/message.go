package bot

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// handleMessage processes plain user messages and advances registration FSM when active.
func (h *Handler) handleMessage(ctx context.Context, msg messenger.IncomingMessage) {
	if msg.User.ID == 0 {
		return
	}

	text := strings.TrimSpace(msg.Text)
	if strings.EqualFold(text, "/menu") {
		h.sendMenu(ctx, msg.ChatID)
		return
	}
	if strings.HasPrefix(text, "/start") {
		h.handleStart(ctx, msg)
		return
	}

	state, ok, err := h.store.GetRegistrationState(ctx, messengerKindFromIdentity(msg.User.Kind), strconv.FormatInt(msg.User.ID, 10))
	if err != nil {
		slog.Error("load registration state failed", "error", err, "telegram_id", msg.User.ID)
		return
	}
	if !ok || state.Step == domain.StepNone || state.Step == domain.StepDone {
		return
	}

	h.handleRegistrationStep(ctx, msg, state)
}
