package bot

import (
	"context"
	"log/slog"
	"strconv"
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
		slog.Error("answer callback failed", "error", err, "callback_id", cb.ID)
	}

	if !strings.HasPrefix(cb.Data, "accept_terms:") {
		if strings.HasPrefix(cb.Data, "pay:") {
			h.handlePay(ctx, cb)
		} else if strings.HasPrefix(cb.Data, menuCallbackPrefix) {
			h.handleMenuCallback(ctx, cb)
		}
		return
	}

	connectorIDRaw := strings.TrimPrefix(cb.Data, "accept_terms:")
	connectorID, err := strconv.ParseInt(connectorIDRaw, 10, 64)
	if err != nil || connectorID <= 0 {
		h.send(ctx, cb.From.ID, "Коннектор не найден или отключен.")
		return
	}
	connector, ok, err := h.store.GetConnector(ctx, connectorID)
	if err != nil {
		slog.Error("load connector failed", "error", err, "connector_id", connectorID)
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
		slog.Error("save consent failed", "error", err, "telegram_id", cb.From.ID, "connector_id", connectorID)
		return
	}
	h.logAuditEvent(ctx, cb.From.ID, connectorID, "consent_accepted", "")

	user, exists, err := h.store.GetUser(ctx, cb.From.ID)
	if err != nil {
		slog.Error("load user before registration flow failed", "error", err, "telegram_id", cb.From.ID, "connector_id", connectorID)
		return
	}
	if !exists {
		user = domain.User{TelegramID: cb.From.ID}
	}
	updatedUsername := applyCurrentTelegramUsername(&user, cb.From.Username)
	if updatedUsername || !exists {
		if err := h.store.SaveUser(ctx, user); err != nil {
			slog.Error("save user before registration flow failed", "error", err, "telegram_id", cb.From.ID, "connector_id", connectorID)
			return
		}
	}

	nextStep := nextRegistrationStep(user)
	if nextStep == domain.StepDone {
		if err := h.store.DeleteRegistrationState(ctx, cb.From.ID); err != nil {
			slog.Error("delete registration state failed", "error", err, "telegram_id", cb.From.ID)
		}
		h.logAuditEvent(ctx, cb.From.ID, connectorID, "registration_reused_existing_profile", "")
		h.sendFinalRegistrationMessage(ctx, cb.From.ID, connectorID)
		return
	}

	state := domain.RegistrationState{
		TelegramID:       cb.From.ID,
		ConnectorID:      connectorID,
		Step:             nextStep,
		TelegramUsername: user.TelegramUsername,
	}
	if err := h.store.SaveRegistrationState(ctx, state); err != nil {
		slog.Error("save registration state failed", "error", err, "telegram_id", cb.From.ID, "connector_id", connectorID)
		return
	}
	h.logAuditEvent(ctx, cb.From.ID, connectorID, "registration_step_requested", string(nextStep))

	h.send(ctx, cb.From.ID, registrationPrompt(nextStep))
}
