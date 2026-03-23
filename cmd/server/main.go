package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Jopoleon/invest-control-bot/internal/app"
	"github.com/Jopoleon/invest-control-bot/internal/bootstrap"
	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/logger"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// main is the backend entrypoint used for local/dev runs and VPS deployments.
func main() {
	// Bootstrap logger before config load, then reconfigure from LOG_LEVEL.
	if _, err := logger.Init("info", ""); err != nil {
		slog.Error("bootstrap logger init failed", "error", err)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}
	if cfg.Runtime != config.RuntimeServer {
		slog.Error("invalid APP_RUNTIME for cmd/server", "runtime", cfg.Runtime, "expected", config.RuntimeServer)
		os.Exit(1)
	}
	effectiveLevel, err := logger.Init(cfg.Logging.Level, cfg.Logging.FilePath)
	if err != nil {
		slog.Error("logger init with file failed", "error", err, "file_path", cfg.Logging.FilePath)
		os.Exit(1)
	}
	slog.Info("config loaded", "config", cfg, "effective_log_level", effectiveLevel, "log_file_path", cfg.Logging.FilePath)

	st, cleanup, err := bootstrap.OpenStore(cfg)
	if err != nil {
		slog.Error("init store failed", "error", err)
		os.Exit(1)
	}
	defer cleanup()

	srv, err := app.New(cfg, st)
	if err != nil {
		slog.Error("create app server failed", "error", err)
		os.Exit(1)
	}

	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("service started", "service", cfg.AppName, "env", cfg.Environment, "http_addr", cfg.HTTP.Address)
	if err := srv.Run(runCtx); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
