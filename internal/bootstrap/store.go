package bootstrap

import (
	"context"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/config"
	"github.com/Jopoleon/invest-control-bot/internal/store"
	"github.com/Jopoleon/invest-control-bot/internal/store/memory"
	postgresstore "github.com/Jopoleon/invest-control-bot/internal/store/postgres"
	"github.com/Jopoleon/invest-control-bot/migrations"
	"github.com/jmoiron/sqlx"
)

// OpenStore initializes storage backend from runtime config.
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
			if _, err := migrations.ApplyUp(db); err != nil {
				_ = db.Close()
				return nil, func() {}, err
			}
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
