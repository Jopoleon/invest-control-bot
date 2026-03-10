package bot

import (
	"context"
	"log/slog"
	"strings"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/go-telegram/bot/models"
)

// handleMessage processes plain user messages and advances registration FSM when active.
func (h *Handler) handleMessage(ctx context.Context, msg *models.Message) {
	if msg == nil || msg.From == nil {
		return
	}

	text := strings.TrimSpace(msg.Text)
	if strings.HasPrefix(text, "/start") {
		h.handleStart(ctx, msg)
		return
	}

	state, ok, err := h.store.GetRegistrationState(ctx, msg.From.ID)
	if err != nil {
		slog.Error("load registration state failed", "error", err, "telegram_id", msg.From.ID)
		return
	}
	if !ok || state.Step == domain.StepNone || state.Step == domain.StepDone {
		return
	}

	h.handleRegistrationStep(ctx, msg, state)
}
