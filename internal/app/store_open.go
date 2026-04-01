package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/store"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
	postgresstore "github.com/Jopoleon/invest-control-bot/internal/store/postgres"
	"github.com/Jopoleon/invest-control-bot/migrations"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

// OpenStore initializes the configured storage backend for app/server entrypoints.
func OpenStore(cfg config.Config) (store.Store, func(), error) {
	switch cfg.Postgres.Driver {
	case "memory":
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
			slog.Info("applying database migrations", "database", cfg.Postgres.Database)
			applied, err := migrations.ApplyUp(db)
			if err != nil {
				_ = db.Close()
				return nil, func() {}, err
			}
			slog.Info("database migrations applied", "database", cfg.Postgres.Database, "applied", applied)
		} else {
			slog.Info("database migrations skipped", "database", cfg.Postgres.Database, "reason", "DB_WITH_MIGRATION=false")
		}
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
