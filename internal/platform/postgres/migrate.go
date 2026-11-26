package postgres

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

var schemaName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Migrate brings one schema up to date. It must be called on the
// administrative handle returned by Open.
//
// Every module owns exactly one schema, its own migration history
// (<schema>.goose_db_version) and its own database role (<schema>_rw), so
// modules evolve their storage independently and can later move to their
// own database without untangling anything shared.
//
// Migrate is idempotent and safe to run from several processes at once: a
// session-level advisory lock serialises them.
func Migrate(ctx context.Context, db *DB, schema string, fsys fs.FS) error {
	// The schema name ends up, unquoted, in SQL and in goose's version table
	// name, so only plain lower-case identifiers are accepted.
	if !schemaName.MatchString(schema) {
		return fmt.Errorf("postgres: invalid schema name %q: want lower-case letters, digits and underscores", schema)
	}
	role := RoleFor(schema)

	if err := ensureRole(ctx, db, role); err != nil {
		return fmt.Errorf("postgres: role %[1]s: %[2]w (the migrating user must be allowed to create roles; "+
			"if %[1]s already exists, its owner must run GRANT %[1]s TO that user)", role, err)
	}
	if _, err := db.pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema); err != nil {
		return fmt.Errorf("postgres: create schema %s: %w", schema, err)
	}
	if err := runMigrations(ctx, db, schema, fsys); err != nil {
		return err
	}

	// The role may use everything in its own schema, now and in future
	// migrations, and nothing anywhere else.
	grants := fmt.Sprintf(`
		GRANT USAGE ON SCHEMA %[1]s TO %[2]s;
		GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA %[1]s TO %[2]s;
		GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA %[1]s TO %[2]s;
		ALTER DEFAULT PRIVILEGES IN SCHEMA %[1]s GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %[2]s;
		ALTER DEFAULT PRIVILEGES IN SCHEMA %[1]s GRANT USAGE, SELECT ON SEQUENCES TO %[2]s;`,
		schema, role)
	if _, err := db.pool.Exec(ctx, grants); err != nil {
		return fmt.Errorf("postgres: grant %s on schema %s: %w", role, schema, err)
	}
	return nil
}

// ensureRole creates the NOLOGIN role and lets the migrating user switch to
// it. Roles are shared by every database of a server, so two migrations
// running against different databases can collide here; the statement is
// idempotent, so it is simply retried.
func ensureRole(ctx context.Context, db *DB, role string) error {
	stmt := fmt.Sprintf(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '%[1]s') THEN
				CREATE ROLE %[1]s NOLOGIN;
			END IF;
			IF NOT pg_has_role(current_user, '%[1]s', 'SET') THEN
				GRANT %[1]s TO CURRENT_USER;
			END IF;
		END
		$$`, role)

	var err error
	for attempt := range 5 {
		if _, err = db.pool.Exec(ctx, stmt); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 50 * time.Millisecond):
		}
	}
	return err
}

func runMigrations(ctx context.Context, db *DB, schema string, fsys fs.FS) error {
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
