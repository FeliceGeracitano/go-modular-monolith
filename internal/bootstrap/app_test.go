package bootstrap_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/bootstrap"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/config"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres/pgtest"
)

// These tests exercise the fully wired application against a real
// PostgreSQL: HTTP in, SQL and events underneath, exactly as in production.
// They are skipped unless TEST_DATABASE_URL is set.
//
// This file holds the harness and the tests that apply to any set of
// modules. Tests for specific modules live next to it.

var allInOne = bootstrap.Roles{API: true, Worker: true}

type testApp struct {
	t       *testing.T
	app     *bootstrap.App
	handler http.Handler
	dbURL   string
}

// newTestApp starts the application against a fresh, migrated database.
func newTestApp(t *testing.T, roles bootstrap.Roles) *testApp {
	t.Helper()
	return startApp(t, pgtest.NewDatabase(t), roles)
}

// startApp starts another process of the application against an existing
// database, the way an api and a worker share one in production.
func startApp(t *testing.T, dbURL string, roles bootstrap.Roles) *testApp {
	t.Helper()
	ctx := context.Background()

	cfg, err := config.LoadFrom(func(key string) (string, bool) {
		if key == "DATABASE_URL" {
			return dbURL, true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	app, err := bootstrap.New(ctx, cfg, slog.New(slog.DiscardHandler), roles)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Cleanup(app.Close)
	if err := app.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Migrating twice must be a no-op: every instance does it on startup.
	if err := app.Migrate(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	return &testApp{t: t, app: app, handler: app.Handler(), dbURL: dbURL}
}

// do sends a JSON request through the real handler chain and decodes the
// JSON response into out (when out is not nil).
func (a *testApp) do(method, path string, body, out any) *httptest.ResponseRecorder {
	a.t.Helper()

	var reader io.Reader
	if body != nil {
		reader = jsonBody(a.t, body)
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, httptest.NewRequest(method, path, reader))

	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			a.t.Fatalf("%s %s: response is not JSON (%v): %s", method, path, err, rec.Body)
		}
	}
	return rec
}

func jsonBody(t *testing.T, v any) io.Reader {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(raw)
}

func TestProbesAreServedByEveryRole(t *testing.T) {
	for name, roles := range map[string]bootstrap.Roles{
		"all in one": allInOne,
		"api":        {API: true},
		"worker":     {Worker: true},
	} {
		t.Run(name, func(t *testing.T) {
			app := newTestApp(t, roles)
			checkProbes(t, app)
		})
	}
}

func checkProbes(t *testing.T, app *testApp) {
	t.Helper()

	for _, path := range []string{"/healthz", "/readyz"} {
		rec := app.do(http.MethodGet, path, nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d: %s", path, rec.Code, rec.Body)
		}
		if rec.Header().Get("X-Request-Id") == "" {
			t.Fatalf("GET %s: missing X-Request-Id", path)
		}
	}
}
