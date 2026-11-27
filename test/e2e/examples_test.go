package e2e_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/telemetry"
)

type customer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type customerList struct {
	Items  []customer `json:"items"`
	Total  int        `json:"total"`
	Limit  int        `json:"limit"`
	Offset int        `json:"offset"`
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

type orderList struct {
	Items  []order `json:"items"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}

type notification struct {
	ID         string `json:"id"`
	EventID    string `json:"event_id"`
	CustomerID string `json:"customer_id"`
	Recipient  string `json:"recipient"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
}

type notificationList struct {
	Items []notification `json:"items"`
	Total int            `json:"total"`
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

func TestOrderLifecycleEndToEnd(t *testing.T) {
	app := newTestApp(t)

	// Customers: register, list, update, delete another.
	var ada, bob customer
	rec := app.do(http.MethodPost, "/api/v1/customers", map[string]string{"name": "Ada Lovelace", "email": "Ada@Example.com"}, &ada)
	if rec.Code != http.StatusCreated || ada.Email != "ada@example.com" || rec.Header().Get("Location") != "/api/v1/customers/"+ada.ID {
		t.Fatalf("register ada: status %d body %s", rec.Code, rec.Body)
	}
	app.do(http.MethodPost, "/api/v1/customers", map[string]string{"name": "Bob", "email": "bob@example.com"}, &bob)

	var cList customerList
	if rec := app.do(http.MethodGet, "/api/v1/customers?limit=1&offset=0", nil, &cList); rec.Code != http.StatusOK || cList.Total != 2 || len(cList.Items) != 1 {
		t.Fatalf("list customers: status %d list %+v", rec.Code, cList)
	}

	var updated customer
	if rec := app.do(http.MethodPatch, "/api/v1/customers/"+ada.ID, map[string]string{"name": "Augusta Ada"}, &updated); rec.Code != http.StatusOK || updated.Name != "Augusta Ada" {
		t.Fatalf("update customer: status %d body %s", rec.Code, rec.Body)
	}

	if rec := app.do(http.MethodDelete, "/api/v1/customers/"+bob.ID, nil, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete customer: status %d", rec.Code)
	}
	if rec := app.do(http.MethodGet, "/api/v1/customers/"+bob.ID, nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("deleted customer still exists: status %d", rec.Code)
	}

	// Orders: place (sync customersapi ACL + outbox OrderPlaced).
	var placed order
	rec = app.do(http.MethodPost, "/api/v1/orders", orderRequest(ada.ID), &placed)
	if rec.Code != http.StatusCreated || placed.TotalAmount != 2999 || placed.Status != "placed" {
		t.Fatalf("place order: status %d body %s", rec.Code, rec.Body)
	}

	var fetched order
	if rec := app.do(http.MethodGet, "/api/v1/orders/"+placed.ID, nil, &fetched); rec.Code != http.StatusOK || fetched.CustomerID != ada.ID {
		t.Fatalf("get order: status %d body %s", rec.Code, rec.Body)
	}

	// Before relay runs, notifications list is empty.
	var notifs notificationList
	app.do(http.MethodGet, "/api/v1/notifications?customer_id="+ada.ID, nil, &notifs)
	if notifs.Total != 0 {
		t.Fatalf("%d notifications before relay ran, want 0", notifs.Total)
	}

	if n, err := app.app.Relay().ProcessBatch(context.Background()); err != nil || n != 1 {
		t.Fatalf("relay handled %d messages (err %v), want 1", n, err)
	}

	// Cancel order (emits orders.order_cancelled.v1) and filter orders by status.
	var cancelled order
	if rec := app.do(http.MethodPost, "/api/v1/orders/"+placed.ID+"/cancel", nil, &cancelled); rec.Code != http.StatusOK || cancelled.Status != "cancelled" {
		t.Fatalf("cancel order: status %d body %s", rec.Code, rec.Body)
	}
	var dup problem
	if rec := app.do(http.MethodPost, "/api/v1/orders/"+placed.ID+"/cancel", nil, &dup); rec.Code != http.StatusConflict || dup.Code != "orders.already_cancelled" {
		t.Fatalf("duplicate cancel: status %d problem %+v", rec.Code, dup)
	}

	var oList orderList
	app.do(http.MethodGet, "/api/v1/orders?customer_id="+ada.ID+"&status=cancelled", nil, &oList)
	if oList.Total != 1 || oList.Items[0].ID != placed.ID {
		t.Fatalf("filter cancelled orders: %+v", oList)
	}

	if n, err := app.app.Relay().ProcessBatch(context.Background()); err != nil || n != 1 {
		t.Fatalf("cancel relay handled %d messages (err %v), want 1", n, err)
	}

	// Both confirmation and cancellation notifications are now observable over HTTP.
	app.do(http.MethodGet, "/api/v1/notifications?customer_id="+ada.ID, nil, &notifs)
	if notifs.Total != 2 {
		t.Fatalf("got %d notifications, want 2: %+v", notifs.Total, notifs.Items)
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

	if n, err := app.app.Relay().ProcessBatch(context.Background()); err != nil || n != 0 {
		t.Fatalf("rejected orders queued %d outbox messages (err %v)", n, err)
	}
}

func TestTraceFollowsTheOrderThroughTheOutbox(t *testing.T) {
	ctx := context.Background()
	recorder := tracetest.NewSpanRecorder()
	if _, err := telemetry.Setup(ctx, telemetry.Config{}); err != nil {
		t.Fatal(err)
	}
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))

	app := newTestApp(t)

	var ada customer
	app.do(http.MethodPost, "/api/v1/customers", map[string]string{"name": "Ada", "email": "ada@example.com"}, &ada)

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
		"POST /api/v1/orders",
		"deliver orders.order_placed.v1",
	} {
		if names[want] == 0 {
			t.Errorf("no %q span in caller's trace; got %v", want, names)
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
