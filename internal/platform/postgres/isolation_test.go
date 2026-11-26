package postgres_test

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres/pgtest"
)

func migration(schema string) fstest.MapFS {
	return fstest.MapFS{
		"00001_init.sql": {Data: []byte("-- +goose Up\nCREATE TABLE " + schema + ".things (id integer PRIMARY KEY);\n")},
	}
}

// A module's database handle can use its own schema and nothing else. This
// is what turns "modules never touch each other's tables" from a convention
// into something the database enforces.
func TestScopedHandleCannotReachOtherSchemas(t *testing.T) {
	ctx := context.Background()
	admin := pgtest.Open(t)

	for _, schema := range []string{"alpha", "beta"} {
		if err := postgres.Migrate(ctx, admin, schema, migration(schema)); err != nil {
			t.Fatalf("migrate %s: %v", schema, err)
		}
	}

	alpha, err := admin.Scoped("alpha")
	if err != nil {
		t.Fatalf("scoped: %v", err)
	}
	t.Cleanup(alpha.Close)

	// Its own schema: full read and write access.
	if _, err := alpha.Conn(ctx).Exec(ctx, `INSERT INTO alpha.things (id) VALUES (1)`); err != nil {
		t.Fatalf("write to own schema: %v", err)
	}
	var n int
	if err := alpha.Conn(ctx).QueryRow(ctx, `SELECT count(*) FROM alpha.things`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("read own schema: n=%d err=%v", n, err)
	}

	// Another module's schema: refused, for reads and writes alike.
	for _, stmt := range []string{
		`SELECT count(*) FROM beta.things`,
		`INSERT INTO beta.things (id) VALUES (1)`,
		`SELECT count(*) FROM alpha.things a JOIN beta.things b USING (id)`,
	} {
		_, err := alpha.Conn(ctx).Exec(ctx, stmt)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Errorf("%s\n\terror = %v, want permission denied (42501)", stmt, err)
		}
	}

	// And it cannot change its own structure: only migrations do that.
	if _, err := alpha.Conn(ctx).Exec(ctx, `CREATE TABLE alpha.sneaky (id integer)`); err == nil {
		t.Error("a scoped handle must not be able to create tables")
	}
}

// Tables added by a later migration are covered without re-granting by hand.
func TestScopedHandleSeesTablesFromLaterMigrations(t *testing.T) {
	ctx := context.Background()
	admin := pgtest.Open(t)

	fsys := migration("gamma")
	if err := postgres.Migrate(ctx, admin, "gamma", fsys); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	fsys["00002_more.sql"] = &fstest.MapFile{Data: []byte("-- +goose Up\nCREATE TABLE gamma.more (id bigserial PRIMARY KEY);\n")}
	if err := postgres.Migrate(ctx, admin, "gamma", fsys); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	gamma, err := admin.Scoped("gamma")
	if err != nil {
		t.Fatalf("scoped: %v", err)
	}
	t.Cleanup(gamma.Close)

	if _, err := gamma.Conn(ctx).Exec(ctx, `INSERT INTO gamma.more DEFAULT VALUES`); err != nil {
		t.Fatalf("insert into a table from a later migration: %v", err)
	}
}

func TestMigrateRejectsUnsafeSchemaNames(t *testing.T) {
	admin := pgtest.Open(t)

	for _, name := range []string{"", "Orders", "orders; DROP SCHEMA public", `"quoted"`, "1st"} {
		if err := postgres.Migrate(context.Background(), admin, name, fstest.MapFS{}); err == nil {
			t.Errorf("Migrate accepted schema name %q", name)
		}
	}
}
