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
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres/pgtest"
)

// These tests exercise the fully wired application against a real
// PostgreSQL: HTTP in, SQL and events underneath, exactly as in production.
// They are skipped unless TEST_DATABASE_URL is set.

type testApp struct {
	t       *testing.T
	app     *bootstrap.App
	handler http.Handler
	dbURL   string
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	ctx := context.Background()

	dbURL := pgtest.NewDatabase(t)
	cfg, err := config.LoadFrom(func(key string) (string, bool) {
		if key == "DATABASE_URL" {
			return dbURL, true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	app, err := bootstrap.New(ctx, cfg, slog.New(slog.DiscardHandler))
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
		raw, err := json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
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

func (a *testApp) count(query string) int {
	a.t.Helper()
	db, err := postgres.Open(context.Background(), a.dbURL, 1)
	if err != nil {
		a.t.Fatal(err)
	}
	defer db.Close()

	var n int
	if err := db.Pool().QueryRow(context.Background(), query).Scan(&n); err != nil {
		a.t.Fatal(err)
	}
	return n
}

type customer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type order struct {
	ID          string `json:"id"`
	CustomerID  string `json:"customer_id"`
	Status      string `json:"status"`
	Currency    string `json:"currency"`
	TotalAmount int64  `json:"total_amount"`
	Lines       []struct {
		SKU       string `json:"sku"`
		Quantity  int64  `json:"quantity"`
		UnitPrice int64  `json:"unit_price"`
	} `json:"lines"`
}

type problem struct {
	Status int    `json:"status"`
	Code   string `json:"code"`
}

func orderRequest(customerID string) map[string]any {
	return map[string]any{
		"customer_id": customerID,
		"currency":    "EUR",
		"lines": []map[string]any{
			{"sku": "BOOK-1", "quantity": 2, "unit_price": 1250},
			{"sku": "PEN-7", "quantity": 1, "unit_price": 499},
		},
	}
}

func TestPlacingAnOrderEndToEnd(t *testing.T) {
	app := newTestApp(t)

	// customers: register.
	var ada customer
	rec := app.do(http.MethodPost, "/api/v1/customers", map[string]string{"name": "Ada Lovelace", "email": "Ada@Example.com"}, &ada)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register customer: status %d: %s", rec.Code, rec.Body)
	}
	if ada.Email != "ada@example.com" || rec.Header().Get("Location") != "/api/v1/customers/"+ada.ID {
		t.Fatalf("unexpected customer response: %+v, Location %q", ada, rec.Header().Get("Location"))
	}

	// orders: place. Synchronously checks the customer through customersapi.
	var placed order
	rec = app.do(http.MethodPost, "/api/v1/orders", orderRequest(ada.ID), &placed)
	if rec.Code != http.StatusCreated {
		t.Fatalf("place order: status %d: %s", rec.Code, rec.Body)
	}
	if placed.TotalAmount != 2999 || placed.Status != "placed" {
		t.Fatalf("unexpected order response: %+v", placed)
	}

	// orders: read back from the database.
	var fetched order
	if rec := app.do(http.MethodGet, "/api/v1/orders/"+placed.ID, nil, &fetched); rec.Code != http.StatusOK {
		t.Fatalf("get order: status %d: %s", rec.Code, rec.Body)
	}
	if fetched.TotalAmount != 2999 || len(fetched.Lines) != 2 || fetched.Lines[0].SKU != "BOOK-1" || fetched.CustomerID != ada.ID {
		t.Fatalf("stored order differs from the placed one: %+v", fetched)
	}

	// notifications: nothing yet, the event is still in the outbox.
	if n := app.count(`SELECT count(*) FROM notifications.notifications`); n != 0 {
		t.Fatalf("%d notifications before the relay ran, want 0", n)
	}

	// Relay the outbox: orders -> (OrderPlaced) -> notifications.
	if n, err := app.app.Relay().ProcessBatch(context.Background()); err != nil || n != 1 {
		t.Fatalf("relay handled %d messages (err %v), want 1", n, err)
	}
	if n := app.count(`SELECT count(*) FROM notifications.notifications WHERE recipient = 'ada@example.com'`); n != 1 {
		t.Fatalf("%d notifications for the customer, want 1", n)
	}
	if n := app.count(`SELECT count(*) FROM outbox.messages WHERE processed_at IS NULL`); n != 0 {
		t.Fatalf("%d messages still pending, want 0", n)
	}
}

func TestErrorsAreReportedAsProblems(t *testing.T) {
	app := newTestApp(t)

	var ada customer
	app.do(http.MethodPost, "/api/v1/customers", map[string]string{"name": "Ada", "email": "ada@example.com"}, &ada)

	tests := []struct {
		name       string
		method     string
		path       string
		body       any
		wantStatus int
		wantCode   string
	}{
		{"duplicate email", http.MethodPost, "/api/v1/customers",
			map[string]string{"name": "Other Ada", "email": "ada@example.com"},
			http.StatusConflict, "customers.email_taken"},
		{"invalid email", http.MethodPost, "/api/v1/customers",
			map[string]string{"name": "Bob", "email": "nope"},
			http.StatusUnprocessableEntity, "customers.invalid_email"},
		{"unknown field", http.MethodPost, "/api/v1/customers",
			map[string]any{"name": "Bob", "email": "bob@example.com", "admin": true},
			http.StatusUnprocessableEntity, "http.malformed_body"},
		{"missing customer", http.MethodGet, "/api/v1/customers/0198c1a0-0000-7000-8000-000000000000", nil,
			http.StatusNotFound, "customers.not_found"},
		{"malformed id", http.MethodGet, "/api/v1/orders/42", nil,
			http.StatusNotFound, "orders.not_found"},
		{"order for unknown customer", http.MethodPost, "/api/v1/orders",
			orderRequest("0198c1a0-0000-7000-8000-000000000000"),
			http.StatusUnprocessableEntity, "orders.unknown_customer"},
		{"order without lines", http.MethodPost, "/api/v1/orders",
			map[string]any{"customer_id": ada.ID, "currency": "EUR", "lines": []any{}},
			http.StatusUnprocessableEntity, "orders.no_lines"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got problem
			rec := app.do(tt.method, tt.path, tt.body, &got)
			if rec.Code != tt.wantStatus || got.Code != tt.wantCode {
				t.Fatalf("got %d %q, want %d %q (body: %s)", rec.Code, got.Code, tt.wantStatus, tt.wantCode, rec.Body)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json; charset=utf-8" {
				t.Fatalf("Content-Type = %q", ct)
			}
		})
	}

	// A rejected order leaves no trace: no row, no event.
	if n := app.count(`SELECT (SELECT count(*) FROM orders.orders) + (SELECT count(*) FROM outbox.messages)`); n != 0 {
		t.Fatalf("rejected orders left %d rows behind", n)
	}
}

func TestProbes(t *testing.T) {
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
