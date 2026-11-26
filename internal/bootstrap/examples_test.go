package bootstrap_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/bootstrap"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/telemetry"
)

// End-to-end tests for the example modules (customers, orders,
// notifications). Delete this file together with them.

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
	app := newTestApp(t, allInOne)

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
	app := newTestApp(t, allInOne)

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

// The api and the worker are separate processes sharing one database: the
// api accepts the order, the worker delivers the event.
func TestAPIAndWorkerSplitTheWork(t *testing.T) {
	api := newTestApp(t, bootstrap.Roles{API: true})
	worker := startApp(t, api.dbURL, bootstrap.Roles{Worker: true})

	var ada customer
	api.do(http.MethodPost, "/api/v1/customers", map[string]string{"name": "Ada", "email": "ada@example.com"}, &ada)
	if rec := api.do(http.MethodPost, "/api/v1/orders", orderRequest(ada.ID), nil); rec.Code != http.StatusCreated {
		t.Fatalf("place order: status %d: %s", rec.Code, rec.Body)
	}

	// The api has no relay, so nothing is delivered until a worker runs.
	if api.app.Relay() != nil {
		t.Fatal("an api-only process must not run the outbox relay")
	}
	// The worker exposes no business endpoints.
	if rec := worker.do(http.MethodGet, "/api/v1/customers/"+ada.ID, nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("worker served an API route: status %d", rec.Code)
	}

	if n, err := worker.app.Relay().ProcessBatch(context.Background()); err != nil || n != 1 {
		t.Fatalf("worker relay handled %d messages (err %v), want 1", n, err)
	}
	if n := api.count(`SELECT count(*) FROM notifications.notifications`); n != 1 {
		t.Fatalf("%d notifications, want 1", n)
	}
}

// Each module's connections run as a database role that can reach only that
// module's schema. The application works end to end under that restriction
// (the tests above), and the restriction is real (this test).
func TestModulesCannotReadEachOthersTables(t *testing.T) {
	app := newTestApp(t, allInOne)
	ctx := context.Background()

	admin, err := postgres.Open(ctx, app.dbURL, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	orders, err := admin.Scoped("orders")
	if err != nil {
		t.Fatal(err)
	}
	defer orders.Close()

	if _, err := orders.Conn(ctx).Exec(ctx, `SELECT 1 FROM orders.orders`); err != nil {
		t.Fatalf("orders cannot read its own table: %v", err)
	}
	for _, stmt := range []string{
		`SELECT 1 FROM customers.customers`,
		`SELECT 1 FROM notifications.notifications`,
		`SELECT 1 FROM outbox.messages`,           // modules may publish...
		`UPDATE outbox.messages SET attempts = 0`, // ...but not read or rewrite the queue
	} {
		if _, err := orders.Conn(ctx).Exec(ctx, stmt); err == nil {
			t.Errorf("the orders role was allowed to run: %s", stmt)
		}
	}
}

// One request produces one trace that covers the HTTP request, its SQL, and
// the event handler that runs later in the worker.
func TestTraceFollowsTheOrderThroughTheOutbox(t *testing.T) {
	ctx := context.Background()
	recorder := tracetest.NewSpanRecorder()
	if _, err := telemetry.Setup(ctx, telemetry.Config{}); err != nil { // installs trace propagation
		t.Fatal(err)
	}
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))

	app := newTestApp(t, allInOne)

	var ada customer
	app.do(http.MethodPost, "/api/v1/customers", map[string]string{"name": "Ada", "email": "ada@example.com"}, &ada)

	// The caller starts the trace, as an upstream service or gateway would.
	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/orders", jsonBody(t, orderRequest(ada.ID)))
	req.Header.Set("traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()
	app.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("place order: status %d: %s", rec.Code, rec.Body)
	}
	if _, err := app.app.Relay().ProcessBatch(ctx); err != nil {
		t.Fatal(err)
	}

	names := map[string]int{}
	for _, span := range recorder.Ended() {
		if span.SpanContext().TraceID().String() == traceID {
			names[span.Name()]++
		}
	}
	for _, want := range []string{
		"POST /api/v1/orders",            // inbound request, named after the route
		"INSERT",                         // its SQL
		"deliver orders.order_placed.v1", // the event, delivered after the request finished
	} {
		if names[want] == 0 {
			t.Errorf("no %q span in the caller's trace; got %v", want, names)
		}
	}
	var inbound trace.SpanKind
	for _, span := range recorder.Ended() {
		if span.Name() == "POST /api/v1/orders" {
			inbound = span.SpanKind()
		}
	}
	if inbound != trace.SpanKindServer {
		t.Errorf("inbound span kind = %v, want server", inbound)
	}
}
