package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/Jopoleon/telega-bot-fedor/internal/app"
	"github.com/Jopoleon/telega-bot-fedor/internal/config"
	"github.com/Jopoleon/telega-bot-fedor/internal/logger"
	"github.com/Jopoleon/telega-bot-fedor/internal/store"
	"github.com/Jopoleon/telega-bot-fedor/internal/store/memory"
	postgresstore "github.com/Jopoleon/telega-bot-fedor/internal/store/postgres"
	"github.com/Jopoleon/telega-bot-fedor/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
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
	effectiveLevel, err := logger.Init(cfg.Logging.Level, cfg.Logging.FilePath)
	if err != nil {
		slog.Error("logger init with file failed", "error", err, "file_path", cfg.Logging.FilePath)
		os.Exit(1)
	}
	slog.Info("config loaded", "config", cfg, "effective_log_level", effectiveLevel, "log_file_path", cfg.Logging.FilePath)

	st, cleanup, err := initStore(cfg)
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

	slog.Info("service started", "service", cfg.AppName, "env", cfg.Environment, "http_addr", cfg.HTTP.Address)
	if err := srv.Run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func initStore(cfg config.Config) (store.Store, func(), error) {
	switch cfg.Postgres.Driver {
	case "memory":
		slog.Warn("DB_DRIVER=memory enabled, data will not persist between restarts")
		return memory.New(), func() {}, nil
	case "postgres":
		db, err := sqlx.Open("pgx", cfg.Postgres.DSN)
		if err != nil {
			return nil, func() {}, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, func() {}, err
		}
		if cfg.Postgres.WithMigration {
			applied, err := migrations.ApplyUp(db)
			if err != nil {
				_ = db.Close()
				return nil, func() {}, err
			}
			slog.Info("postgres migrations applied", "count", applied)
		}
		slog.Info("postgres connection is ready")
		return postgresstore.New(db), func() { _ = db.Close() }, nil
	default:
		return nil, func() {}, &unsupportedDBDriverError{Driver: cfg.Postgres.Driver}
	}
}

type unsupportedDBDriverError struct {
	Driver string
}

func (e *unsupportedDBDriverError) Error() string {
	return "unsupported DB_DRIVER: " + e.Driver
}
