package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/FeliceGeracitano/go-modular-monolith/app"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/config"
)

type testApp struct {
	t       *testing.T
	app     *app.App
	handler http.Handler
	logs    *logCapture
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()

	cfg, err := config.LoadFrom(func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	logs := &logCapture{}
	a, err := app.New(cfg, slog.New(logs))
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	return &testApp{t: t, app: a, handler: a.Handler(), logs: logs}
}

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

type logEntry struct {
	Message string
	Attrs   map[string]string
}

type logCapture struct {
	mu      sync.Mutex
	entries []logEntry
}

func (c *logCapture) Enabled(context.Context, slog.Level) bool { return true }

func (c *logCapture) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]string{}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})
	c.mu.Lock()
	c.entries = append(c.entries, logEntry{Message: r.Message, Attrs: attrs})
	c.mu.Unlock()
	return nil
}

func (c *logCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *logCapture) WithGroup(string) slog.Handler      { return c }

func (c *logCapture) count(msg, key, value string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, e := range c.entries {
		if e.Message == msg && (key == "" || e.Attrs[key] == value) {
			n++
		}
	}
	return n
}

func TestProbesAreServed(t *testing.T) {
	app := newTestApp(t)

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

func TestOpenAPIDocsAreServed(t *testing.T) {
	app := newTestApp(t)

	for _, tt := range []struct {
		path string
		ct   string
	}{
		{"/openapi.yaml", "application/yaml; charset=utf-8"},
		{"/docs", "text/html; charset=utf-8"},
	} {
		rec := app.do(http.MethodGet, tt.path, nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d", tt.path, rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); got != tt.ct {
			t.Fatalf("GET %s: Content-Type = %q, want %q", tt.path, got, tt.ct)
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("GET %s: empty body", tt.path)
		}
	}
}
