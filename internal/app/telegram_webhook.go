package app

import (
	"encoding/json"
	"net/http"

	"github.com/go-telegram/bot/models"
)

func (a *application) handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if a.config.Telegram.Webhook.SecretToken != "" && r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != a.config.Telegram.Webhook.SecretToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var update models.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid update payload"))
		return
	}

	logged := false
	if update.Message != nil {
		raw, _ := json.Marshal(update.Message)
		logDebug("telegram update.message", "payload", string(raw))
		logged = true
	}
	if update.CallbackQuery != nil {
		raw, _ := json.Marshal(update.CallbackQuery.Message)
		logDebug("telegram update.callback_query.message", "payload", string(raw))
		logged = true
	}
	if update.ChannelPost != nil {
		raw, _ := json.Marshal(update.ChannelPost)
		logDebug("telegram update.channel_post", "payload", string(raw))
		logged = true
	}
	if update.EditedChannelPost != nil {
		raw, _ := json.Marshal(update.EditedChannelPost)
		logDebug("telegram update.edited_channel_post", "payload", string(raw))
		logged = true
	}
	if !logged {
		raw, _ := json.Marshal(update)
		logDebug("telegram update.raw", "payload", string(raw))
	}

	a.telegramBotHandler.HandleUpdate(r.Context(), &update)
	w.WriteHeader(http.StatusOK)
}
