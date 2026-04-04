package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Jopoleon/invest-control-bot/internal/app"
	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/logger"
	"github.com/Jopoleon/invest-control-bot/internal/store"
)

// main is the backend entrypoint used for local/dev runs and VPS deployments.
func main() {
	os.Exit(runServerMain(defaultServerDeps()))
}

type serverRunner interface {
	Run(context.Context) error
}

type serverDeps struct {
	initLogger    func(string, string) (string, error)
	loadConfig    func() (config.Config, error)
	openStore     func(config.Config) (store.Store, func(), error)
	newServer     func(config.Config, store.Store) (serverRunner, error)
	notifyContext func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)
}

func defaultServerDeps() serverDeps {
	return serverDeps{
		initLogger: logger.Init,
		loadConfig: config.Load,
		openStore:  app.OpenStore,
		newServer: func(cfg config.Config, st store.Store) (serverRunner, error) {
			return app.New(cfg, st)
		},
		notifyContext: signal.NotifyContext,
	}
}

func runServerMain(deps serverDeps) int {
	// Bootstrap logger before config load, then reconfigure from LOG_LEVEL.
	if _, err := deps.initLogger("info", ""); err != nil {
		slog.Error("bootstrap logger init failed", "error", err)
	}

	cfg, err := deps.loadConfig()
	if err != nil {
		slog.Error("load config failed", "error", err)
		return 1
	}
	if cfg.Runtime != config.RuntimeServer {
		slog.Error("invalid APP_RUNTIME for cmd/server", "runtime", cfg.Runtime, "expected", config.RuntimeServer)
		return 1
	}
	if _, err := deps.initLogger(cfg.Logging.Level, cfg.Logging.FilePath); err != nil {
		slog.Error("logger init with file failed", "error", err, "file_path", cfg.Logging.FilePath)
		return 1
	}

	st, cleanup, err := deps.openStore(cfg)
	if err != nil {
		slog.Error("init store failed", "error", err)
		return 1
	}
	defer cleanup()

	srv, err := deps.newServer(cfg, st)
	if err != nil {
		slog.Error("create app server failed", "error", err)
		return 1
	}

	runCtx, stop := deps.notifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("service started", "service", cfg.AppName, "env", cfg.Environment, "http_addr", cfg.HTTP.Address)
	if err := srv.Run(runCtx); err != nil {
		slog.Error("server stopped", "error", err)
		return 1
	}
	return 0
}
