package handler

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/Jopoleon/invest-control-bot/internal/app"
	"github.com/Jopoleon/invest-control-bot/internal/bootstrap"
	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/logger"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isAllowedCronRequest(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if _, err := logger.Init("info", ""); err != nil {
		http.Error(w, fmt.Sprintf("init logger: %v", err), http.StatusInternalServerError)
		return
	}
	cfg, err := config.Load()
	if err != nil {
		http.Error(w, fmt.Sprintf("load config: %v", err), http.StatusInternalServerError)
		return
	}
	if cfg.Runtime != config.RuntimeVercel {
		http.Error(w, fmt.Sprintf("APP_RUNTIME must be %q for Vercel cron handler, got %q", config.RuntimeVercel, cfg.Runtime), http.StatusInternalServerError)
		return
	}
	if _, err := logger.Init(cfg.Logging.Level, ""); err != nil {
		http.Error(w, fmt.Sprintf("init logger: %v", err), http.StatusInternalServerError)
		return
	}
	st, cleanup, err := bootstrap.OpenStore(cfg)
	if err != nil {
		http.Error(w, fmt.Sprintf("open store: %v", err), http.StatusInternalServerError)
		return
	}
	defer cleanup()
	if err := app.RunLifecyclePassOnce(context.Background(), cfg, st); err != nil {
		http.Error(w, fmt.Sprintf("run lifecycle: %v", err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func isAllowedCronRequest(r *http.Request) bool {
	secret := strings.TrimSpace(os.Getenv("VERCEL_CRON_SECRET"))
	if secret != "" && r.URL.Query().Get("token") == secret {
		return true
	}
	return r.Header.Get("User-Agent") == "vercel-cron/1.0"
}
