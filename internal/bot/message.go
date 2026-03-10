package bot

import (
	"context"
	"log"
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
		log.Printf("load registration state failed: %v", err)
		return
	}
	if !ok || state.Step == domain.StepNone || state.Step == domain.StepDone {
		return
	}

	h.handleRegistrationStep(ctx, msg, state)
}
