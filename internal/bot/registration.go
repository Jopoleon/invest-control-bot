package bot

import (
	"context"
	"log/slog"
	"net/mail"
	"strconv"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/domain"
	"github.com/Jopoleon/invest-control-bot/internal/messenger"
)

// handleRegistrationStep advances onboarding FSM and persists user fields step-by-step.
func (h *Handler) handleRegistrationStep(ctx context.Context, msg messenger.IncomingMessage, state domain.RegistrationState) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		h.sendTo(ctx, msg.ChatID, msg.User, botMsgEmptyValue)
		return
	}

	user, ok := h.resolveMessengerUser(ctx, msg.User)
	if !ok {
		return
	}

	switch state.Step {
	case domain.StepFullName:
		user.FullName = text
		h.logAuditEvent(ctx, msg.User, state.ConnectorID, domain.AuditActionRegistrationFullNameSaved, "")
	case domain.StepPhone:
		phone := normalizePhone(text)
		if !isValidE164(phone) {
			h.sendTo(ctx, msg.ChatID, msg.User, botMsgInvalidPhone)
			return
		}
		user.Phone = phone
		h.logAuditEvent(ctx, msg.User, state.ConnectorID, domain.AuditActionRegistrationPhoneSaved, "")
	case domain.StepEmail:
		if _, err := mail.ParseAddress(text); err != nil {
			h.sendTo(ctx, msg.ChatID, msg.User, botMsgInvalidEmail)
			return
		}
		user.Email = text
		h.logAuditEvent(ctx, msg.User, state.ConnectorID, domain.AuditActionRegistrationEmailSaved, "")
	case domain.StepUsername:
		if text != "-" {
			state.Username = normalizeMessengerUsername(text)
			if _, _, err := h.store.GetOrCreateUserByMessenger(
				ctx,
				messengerKindFromIdentity(msg.User.Kind),
				strconv.FormatInt(msg.User.ID, 10),
				state.Username,
			); err != nil {
				slog.Error("refresh messenger username failed", "error", err, "messenger_kind", msg.User.Kind, "messenger_user_id", msg.User.ID)
				return
			}
		}
	default:
		return
	}

	if err := h.store.SaveUser(ctx, user); err != nil {
		slog.Error("save user failed", "error", err, "messenger_kind", msg.User.Kind, "messenger_user_id", msg.User.ID)
		return
	}

	state.Step = nextRegistrationStep(user, state.Username)
	if state.Step == domain.StepDone {
		if err := h.store.DeleteRegistrationState(ctx, messengerKindFromIdentity(msg.User.Kind), strconv.FormatInt(msg.User.ID, 10)); err != nil {
			slog.Error("delete registration state failed", "error", err, "messenger_kind", msg.User.Kind, "messenger_user_id", msg.User.ID)
		}
		h.sendFinalRegistrationMessage(ctx, msg.ChatID, msg.User, state.ConnectorID)
		h.logAuditEvent(ctx, msg.User, state.ConnectorID, domain.AuditActionRegistrationCompleted, "")
		return
	}

	if err := h.store.SaveRegistrationState(ctx, state); err != nil {
		slog.Error("save registration step failed", "error", err, "messenger_kind", msg.User.Kind, "messenger_user_id", msg.User.ID, "step", state.Step)
		return
	}
	h.sendTo(ctx, msg.ChatID, msg.User, registrationPrompt(state.Step))
}

// sendFinalRegistrationMessage sends completion text and pay button.
func (h *Handler) sendFinalRegistrationMessage(ctx context.Context, chatID int64, userIdentity messenger.UserIdentity, connectorID int64) {
	if handled := h.sendExistingSubscriptionMessage(ctx, chatID, userIdentity, connectorID); handled {
		return
	}
	text, payKeyboard := h.buildFinalPaymentStep(ctx, connectorID, userIdentity.Kind, false)
	if err := h.sender.Send(ctx, recipientRef(chatID, userIdentity), messenger.OutgoingMessage{Text: text, Buttons: payKeyboard}); err != nil {
		slog.Error("send final message failed", "error", err, "chat_id", chatID, "connector_id", connectorID)
	}
}
