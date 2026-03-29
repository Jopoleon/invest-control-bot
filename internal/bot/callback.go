package bot

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// handleCallback routes normalized button actions to the corresponding use-case
// branch. By the time we get here, transport-specific callback parsing is
// already done by the adapter layer.
func (h *Handler) handleCallback(ctx context.Context, cb messenger.IncomingAction) {
	if cb.User.ID == 0 {
		return
	}

	if err := h.sender.AnswerAction(ctx, cb.Ref, ""); err != nil {
		slog.Error("answer callback failed", "error", err, "callback_id", cb.Ref.ID)
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
		h.send(ctx, cb.ChatID, botMsgConnectorUnavailable)
		return
	}
	connector, ok, err := h.store.GetConnector(ctx, connectorID)
	if err != nil {
		slog.Error("load connector failed", "error", err, "connector_id", connectorID)
		return
	}
	if !ok || !connector.IsActive {
		h.send(ctx, cb.ChatID, botMsgConnectorUnavailable)
		return
	}

	user, ok := h.resolveMessengerUser(ctx, cb.User)
	if !ok {
		return
	}

	consent := domain.Consent{
		UserID:            user.ID,
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
		slog.Error("save consent failed", "error", err, "user_id", user.ID, "connector_id", connectorID)
		return
	}
	h.logAuditEvent(ctx, cb.User, connectorID, domain.AuditActionConsentAccepted, "")

	currentUsername := normalizeMessengerUsername(cb.User.Username)
	nextStep := nextRegistrationStep(user, currentUsername)
	if nextStep == domain.StepDone {
		if err := h.store.DeleteRegistrationState(ctx, messengerKindFromIdentity(cb.User.Kind), strconv.FormatInt(cb.User.ID, 10)); err != nil {
			slog.Error("delete registration state failed", "error", err, "telegram_id", cb.User.ID)
		}
		h.logAuditEvent(ctx, cb.User, connectorID, domain.AuditActionRegistrationReusedProfile, "")
		h.sendFinalRegistrationMessage(ctx, cb.ChatID, cb.User.ID, connectorID)
		return
	}

	state := domain.RegistrationState{
		MessengerKind:   messengerKindFromIdentity(cb.User.Kind),
		MessengerUserID: strconv.FormatInt(cb.User.ID, 10),
		ConnectorID:     connectorID,
		Step:            nextStep,
		Username:        currentUsername,
	}
	if err := h.store.SaveRegistrationState(ctx, state); err != nil {
		slog.Error("save registration state failed", "error", err, "telegram_id", cb.User.ID, "connector_id", connectorID)
		return
	}
	h.logAuditEvent(ctx, cb.User, connectorID, domain.AuditActionRegistrationStepRequested, string(nextStep))

	h.send(ctx, cb.ChatID, registrationPrompt(nextStep))
}

// handlePayConsentToggle re-renders the final checkout step with the currently
// selected recurring opt-in state, but does not create a payment yet.
func (h *Handler) handlePayConsentToggle(ctx context.Context, cb messenger.IncomingAction) {
	if cb.ChatID == 0 {
		return
	}

	connectorID, enabled, ok := parsePayConsentCallbackData(cb.Data)
	if !ok {
		h.send(ctx, cb.ChatID, botMsgPayConsentToggleFailed)
		return
	}
	connector, found, err := h.store.GetConnector(ctx, connectorID)
	if err != nil {
		slog.Error("load connector for pay consent toggle failed", "error", err, "connector_id", connectorID)
		return
	}
	if !found || !connector.IsActive {
		h.send(ctx, cb.ChatID, botMsgConnectorUnavailable)
		return
	}

	text, keyboard := h.buildFinalPaymentStep(ctx, connectorID, enabled)
	if err := h.respondToAction(ctx, cb, messenger.OutgoingMessage{
		Text:    text,
		Buttons: keyboard,
	}); err != nil {
		slog.Error("render pay consent message failed", "error", err, "chat_id", cb.ChatID, "message_id", cb.MessageID)
		return
	}
}
