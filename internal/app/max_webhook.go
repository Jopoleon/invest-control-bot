package app

import (
	"encoding/json"
	"net/http"

	"github.com/Jopoleon/invest-control-bot/internal/max"
)

func (a *application) handleMAXWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if a.config.MAX.Webhook.SecretToken != "" && r.Header.Get("X-Max-Bot-Api-Secret") != a.config.MAX.Webhook.SecretToken {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var update max.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid update payload"))
		return
	}

	logDebug("max webhook update", "update_type", update.UpdateType, "payload", string(update.Raw))
	if a.maxAdapter != nil {
		a.maxAdapter.Dispatch(r.Context(), update)
	}
	w.WriteHeader(http.StatusOK)
}
