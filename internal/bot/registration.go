package bot

import (
	"context"
	"log"
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
		log.Printf("load user failed: %v", err)
		return
	}
	if !exists {
		user = domain.User{TelegramID: msg.From.ID, TelegramUsername: msg.From.Username}
	}
	if msg.From.Username != "" {
		user.TelegramUsername = msg.From.Username
	}

	switch state.Step {
	case domain.StepFullName:
		user.FullName = text
		state.Step = domain.StepPhone
		h.send(ctx, msg.Chat.ID, "Телефон")
	case domain.StepPhone:
		phone := normalizePhone(text)
		if !isValidE164(phone) {
			h.send(ctx, msg.Chat.ID, "⚠️ Не правильный телефон. Введите номер в международном формате.")
			return
		}
		user.Phone = phone
		state.Step = domain.StepEmail
		h.send(ctx, msg.Chat.ID, "E-mail")
	case domain.StepEmail:
		if _, err := mail.ParseAddress(text); err != nil {
			h.send(ctx, msg.Chat.ID, "⚠️ Неправильный e-mail")
			return
		}
		user.Email = text
		state.Step = domain.StepUsername
		h.send(ctx, msg.Chat.ID, "Ник телеграм")
	case domain.StepUsername:
		if text != "-" {
			user.TelegramUsername = strings.TrimPrefix(text, "@")
		}
		state.Step = domain.StepDone
		if err := h.store.DeleteRegistrationState(ctx, msg.From.ID); err != nil {
			log.Printf("delete registration state failed: %v", err)
		}

		h.sendFinalRegistrationMessage(ctx, msg.Chat.ID, state.ConnectorID)
	default:
		return
	}

	if err := h.store.SaveUser(ctx, user); err != nil {
		log.Printf("save user failed: %v", err)
		return
	}

	if state.Step != domain.StepDone {
		if err := h.store.SaveRegistrationState(ctx, state); err != nil {
			log.Printf("save registration step failed: %v", err)
		}
	}
}

// sendFinalRegistrationMessage sends completion text and pay button.
func (h *Handler) sendFinalRegistrationMessage(ctx context.Context, chatID int64, connectorID string) {
	payKeyboard := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: "Оплатить", CallbackData: "pay:" + connectorID},
	}}}
	if err := h.tg.SendMessage(ctx, chatID,
		"✅ Спасибо! Ваша заявка оформлена успешно.\n💳 Осталось оплатить\nЧтобы произвести оплату, нажмите на кнопку «Оплатить» ниже, для переадресации на платежную страницу",
		payKeyboard,
	); err != nil {
		log.Printf("send final message failed: %v", err)
	}
}
