package bot

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
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
		} else if strings.HasPrefix(cb.Data, payConsentCallbackPrefix) {
			h.handlePayConsentToggle(ctx, cb)
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
	if connector.OfferURL == "" {
		if doc, found := h.resolveLegalDocument(ctx, domain.LegalDocumentTypeOffer); found {
			consent.OfferDocumentID = doc.ID
			consent.OfferDocumentVersion = doc.Version
		}
	}
	if connector.PrivacyURL == "" {
		if doc, found := h.resolveLegalDocument(ctx, domain.LegalDocumentTypePrivacy); found {
			consent.PrivacyDocumentID = doc.ID
			consent.PrivacyDocumentVersion = doc.Version
		}
	}
	if err := h.store.SaveConsent(ctx, consent); err != nil {
		slog.Error("save consent failed", "error", err, "telegram_id", cb.From.ID, "connector_id", connectorID)
		return
	}
	h.logAuditEvent(ctx, cb.From.ID, connectorID, domain.AuditActionConsentAccepted, "")

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
		h.logAuditEvent(ctx, cb.From.ID, connectorID, domain.AuditActionRegistrationReusedProfile, "")
		h.sendFinalRegistrationMessage(ctx, cb.From.ID, cb.From.ID, connectorID)
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
	h.logAuditEvent(ctx, cb.From.ID, connectorID, domain.AuditActionRegistrationStepRequested, string(nextStep))

	h.send(ctx, cb.From.ID, registrationPrompt(nextStep))
}

func (h *Handler) handlePayConsentToggle(ctx context.Context, cb *models.CallbackQuery) {
	if cb == nil || cb.Message.Message == nil {
		return
	}

	connectorID, enabled, ok := parsePayConsentCallbackData(cb.Data)
	if !ok {
		h.send(ctx, cb.From.ID, "Не удалось обновить настройку автоплатежа.")
		return
	}
	connector, found, err := h.store.GetConnector(ctx, connectorID)
	if err != nil {
		slog.Error("load connector for pay consent toggle failed", "error", err, "connector_id", connectorID)
		return
	}
	if !found || !connector.IsActive {
		h.send(ctx, cb.From.ID, "Коннектор не найден или отключен.")
		return
	}

	text, keyboard := h.buildFinalPaymentStep(ctx, connectorID, enabled)
	if err := h.tg.EditMessageText(ctx, cb.Message.Message.Chat.ID, cb.Message.Message.ID, text, keyboard); err != nil {
		slog.Error("edit pay consent message failed", "error", err, "chat_id", cb.Message.Message.Chat.ID, "message_id", cb.Message.Message.ID)
		return
	}
}
