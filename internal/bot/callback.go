package bot

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/go-telegram/bot/models"
)

// handleCallback handles consent acceptance and pay-button callbacks.
func (h *Handler) handleCallback(ctx context.Context, cb *models.CallbackQuery) {
	if cb == nil {
		return
	}

	if err := h.tg.AnswerCallbackQuery(ctx, cb.ID); err != nil {
		log.Printf("answer callback failed: %v", err)
	}

	if !strings.HasPrefix(cb.Data, "accept_terms:") {
		if strings.HasPrefix(cb.Data, "pay:") {
			h.handlePay(ctx, cb)
		}
		return
	}

	connectorID := strings.TrimPrefix(cb.Data, "accept_terms:")
	connector, ok, err := h.store.GetConnector(ctx, connectorID)
	if err != nil {
		log.Printf("load connector failed: %v", err)
		return
	}
	if !ok || !connector.IsActive {
		h.send(ctx, cb.From.ID, "Коннектор не найден или отключен.")
		return
	}

	consent := domain.Consent{
		TelegramID:        cb.From.ID,
		ConnectorID:       connectorID,
		OfferAcceptedAt:   time.Now().UTC(),
		PrivacyAcceptedAt: time.Now().UTC(),
	}
	if err := h.store.SaveConsent(ctx, consent); err != nil {
		log.Printf("save consent failed: %v", err)
		return
	}

	state := domain.RegistrationState{
		TelegramID:       cb.From.ID,
		ConnectorID:      connectorID,
		Step:             domain.StepFullName,
		TelegramUsername: cb.From.Username,
	}
	if err := h.store.SaveRegistrationState(ctx, state); err != nil {
		log.Printf("save registration state failed: %v", err)
		return
	}

	h.send(ctx, cb.From.ID, "ФИО")
}
