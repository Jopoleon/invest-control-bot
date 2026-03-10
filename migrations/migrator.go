package migrations

import (
	"embed"
	"fmt"
	"net/http"

	"github.com/jmoiron/sqlx"
	migrate "github.com/rubenv/sql-migrate"
)

//go:embed *.sql
var sqlFiles embed.FS

const migrationsTable = "schema_migrations"

func migrationSet() *migrate.MigrationSet {
	return &migrate.MigrationSet{TableName: migrationsTable}
}

func source() *migrate.HttpFileSystemMigrationSource {
	return &migrate.HttpFileSystemMigrationSource{FileSystem: http.FS(sqlFiles)}
}

// ApplyUp executes all pending up-migrations and stores metadata in schema_migrations.
func ApplyUp(db *sqlx.DB) (int, error) {
	applied, err := migrationSet().Exec(db.DB, "postgres", source(), migrate.Up)
	if err != nil {
		return 0, fmt.Errorf("apply up migrations: %w", err)
	}
	return applied, nil
}

// ApplyDown rolls back one migration by default (max=1). For deeper rollback pass bigger max.
func ApplyDown(db *sqlx.DB, max int) (int, error) {
	if max <= 0 {
		max = 1
	}
	applied, err := migrationSet().ExecMax(db.DB, "postgres", source(), migrate.Down, max)
	if err != nil {
		return 0, fmt.Errorf("apply down migrations: %w", err)
	}
	return applied, nil
}
