package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/Jopoleon/invest-control-bot/internal/app"
	"github.com/Jopoleon/invest-control-bot/internal/bootstrap"
	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/logger"
)

var (
	handlerOnce sync.Once
	httpHandler http.Handler
	handlerErr  error
)

func Handler(w http.ResponseWriter, r *http.Request) {
	h, err := getHandler()
	if err != nil {
		http.Error(w, fmt.Sprintf("bootstrap handler: %v", err), http.StatusInternalServerError)
		return
	}
	h.ServeHTTP(w, r)
}

func getHandler() (http.Handler, error) {
	handlerOnce.Do(func() {
		if _, err := logger.Init("info", ""); err != nil {
			handlerErr = err
			return
		}
		cfg, err := config.Load()
		if err != nil {
			handlerErr = err
			return
		}
		if cfg.Runtime != config.RuntimeVercel {
			handlerErr = fmt.Errorf("APP_RUNTIME must be %q for Vercel handler, got %q", config.RuntimeVercel, cfg.Runtime)
			return
		}
		logFilePath := cfg.Logging.FilePath
		if cfg.Runtime == config.RuntimeVercel || os.Getenv("VERCEL") == "1" {
			logFilePath = ""
		}
		if _, err := logger.Init(cfg.Logging.Level, logFilePath); err != nil {
			handlerErr = err
			return
		}
		st, _, err := bootstrap.OpenStore(cfg)
		if err != nil {
			handlerErr = err
			return
		}
		slog.Info("vercel http handler bootstrap complete")
		httpHandler, handlerErr = app.NewHTTPHandler(cfg, st)
	})
	return httpHandler, handlerErr
}
