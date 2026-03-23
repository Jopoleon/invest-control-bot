package handler

import (
	"net/http"

	"github.com/Jopoleon/invest-control-bot/pkg/vercelapp"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	vercelapp.LifecycleHandler(w, r)
}
