// Package pgtest provides throwaway PostgreSQL databases for integration
// tests.
package pgtest

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/id"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres"
)

// EnvVar names the environment variable holding the URL of a PostgreSQL
// server the tests may create databases on.
const EnvVar = "TEST_DATABASE_URL"

// NewDatabase creates an empty database that lives for the duration of the
// test and returns its URL. Every test gets its own database, so tests are
// isolated and can run in parallel.
//
// The test is skipped when TEST_DATABASE_URL is not set, which keeps
// `go test ./...` green on a machine without PostgreSQL.
func NewDatabase(t *testing.T) string {
	t.Helper()

	adminURL := os.Getenv(EnvVar)
	if adminURL == "" {
		t.Skipf("%s is not set; skipping integration test", EnvVar)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("pgtest: connect to %s: %v", EnvVar, err)
	}
	defer func() { _ = admin.Close(ctx) }()

	name := "test_" + strings.ReplaceAll(id.New(), "-", "")
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatalf("pgtest: create database: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, adminURL)
		if err != nil {
			t.Logf("pgtest: connect for cleanup: %v", err)
			return
		}
		defer func() { _ = conn.Close(ctx) }()
		if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
			t.Logf("pgtest: drop database: %v", err)
		}
	})

	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("pgtest: parse %s: %v", EnvVar, err)
	}
	u.Path = "/" + name
	return u.String()
}

// Open creates a throwaway database and returns a connection pool to it.
func Open(t *testing.T) *postgres.DB {
	t.Helper()

	db, err := postgres.Open(context.Background(), NewDatabase(t), 5)
	if err != nil {
		t.Fatalf("pgtest: open: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}
