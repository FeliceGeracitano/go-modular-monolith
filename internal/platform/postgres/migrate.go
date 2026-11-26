package postgres

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

var schemaName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Migrate applies the pending migrations found in fsys to the given schema.
//
// Every module owns exactly one schema and its own migration history
// (<schema>.goose_db_version), so modules evolve their storage independently
// and can later be moved to their own database without untangling a shared
// history. A session-level advisory lock makes concurrent startups safe.
func Migrate(ctx context.Context, db *DB, schema string, fsys fs.FS) error {
	// The schema name ends up, unquoted, in SQL and in goose's version table
	// name, so only plain lower-case identifiers are accepted.
	if !schemaName.MatchString(schema) {
		return fmt.Errorf("postgres: invalid schema name %q: want lower-case letters, digits and underscores", schema)
	}
	if _, err := db.pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema); err != nil {
		return fmt.Errorf("postgres: create schema %s: %w", schema, err)
	}

	sqlDB := stdlib.OpenDBFromPool(db.pool)
	defer func() { _ = sqlDB.Close() }() // closes the adapter, not the pool

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("postgres: migration locker: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, fsys,
		goose.WithTableName(schema+".goose_db_version"),
		goose.WithSessionLocker(locker),
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		if errors.Is(err, goose.ErrNoMigrations) {
			return nil
		}
		return fmt.Errorf("postgres: migrations for %s: %w", schema, err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("postgres: migrate %s: %w", schema, err)
	}
	return nil
}
