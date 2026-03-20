package bot

import (
	"context"
	"log/slog"
	"net/mail"
	"strings"

	"github.com/Jopoleon/telega-bot-fedor/internal/domain"
	"github.com/go-telegram/bot/models"
)

// handleRegistrationStep advances onboarding FSM and persists user fields step-by-step.
func (h *Handler) handleRegistrationStep(ctx context.Context, msg *models.Message, state domain.RegistrationState) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		h.send(ctx, msg.Chat.ID, "Пустое значение. Попробуйте еще раз.")
		return
	}

	user, exists, err := h.store.GetUser(ctx, msg.From.ID)
	if err != nil {
		slog.Error("load user failed", "error", err, "telegram_id", msg.From.ID)
		return
	}
	if !exists {
		user = domain.User{TelegramID: msg.From.ID}
	}
	applyCurrentTelegramUsername(&user, msg.From.Username)

	switch state.Step {
	case domain.StepFullName:
		user.FullName = text
		h.logAuditEvent(ctx, msg.From.ID, state.ConnectorID, domain.AuditActionRegistrationFullNameSaved, "")
	case domain.StepPhone:
		phone := normalizePhone(text)
		if !isValidE164(phone) {
			h.send(ctx, msg.Chat.ID, "⚠️ Не правильный телефон. Введите номер в международном формате.")
			return
		}
		user.Phone = phone
		h.logAuditEvent(ctx, msg.From.ID, state.ConnectorID, domain.AuditActionRegistrationPhoneSaved, "")
	case domain.StepEmail:
		if _, err := mail.ParseAddress(text); err != nil {
			h.send(ctx, msg.Chat.ID, "⚠️ Неправильный e-mail")
			return
		}
		user.Email = text
		h.logAuditEvent(ctx, msg.From.ID, state.ConnectorID, domain.AuditActionRegistrationEmailSaved, "")
	case domain.StepUsername:
		if text != "-" {
			user.TelegramUsername = normalizeTelegramUsername(text)
		}
	default:
		return
	}

	if err := h.store.SaveUser(ctx, user); err != nil {
		slog.Error("save user failed", "error", err, "telegram_id", msg.From.ID)
		return
	}

	state.Step = nextRegistrationStep(user)
	if state.Step == domain.StepDone {
		if err := h.store.DeleteRegistrationState(ctx, msg.From.ID); err != nil {
			slog.Error("delete registration state failed", "error", err, "telegram_id", msg.From.ID)
		}
		h.sendFinalRegistrationMessage(ctx, msg.Chat.ID, state.ConnectorID)
		h.logAuditEvent(ctx, msg.From.ID, state.ConnectorID, domain.AuditActionRegistrationCompleted, "")
		return
	}

	if err := h.store.SaveRegistrationState(ctx, state); err != nil {
		slog.Error("save registration step failed", "error", err, "telegram_id", msg.From.ID, "step", state.Step)
		return
	}
	h.send(ctx, msg.Chat.ID, registrationPrompt(state.Step))
}

// sendFinalRegistrationMessage sends completion text and pay button.
func (h *Handler) sendFinalRegistrationMessage(ctx context.Context, chatID, connectorID int64) {
	text, payKeyboard := h.buildFinalPaymentStep(ctx, connectorID, false)
	if err := h.tg.SendMessage(ctx, chatID, text, payKeyboard); err != nil {
		slog.Error("send final message failed", "error", err, "chat_id", chatID, "connector_id", connectorID)
	}
}
