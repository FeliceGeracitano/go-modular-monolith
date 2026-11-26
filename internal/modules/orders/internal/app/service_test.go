package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/event"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/id"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders/internal/app"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders/internal/domain"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders/ordersapi"
)

// The use cases are tested against in-memory fakes of their ports: fast,
// deterministic and independent of any infrastructure.

type fakeRepo struct {
	orders    map[string]domain.Order
	insertErr error
}

func (f *fakeRepo) Insert(_ context.Context, o domain.Order) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	f.orders[o.ID] = o
	return nil
}

func (f *fakeRepo) ByID(_ context.Context, id string) (domain.Order, error) {
	o, ok := f.orders[id]
	if !ok {
		return domain.Order{}, domain.ErrNotFound
	}
	return o, nil
}

type fakeCustomers map[string]bool

func (f fakeCustomers) Exists(_ context.Context, id string) (bool, error) { return f[id], nil }

type fakePublisher struct{ published []event.Event }

func (f *fakePublisher) Publish(_ context.Context, events ...event.Event) error {
	f.published = append(f.published, events...)
	return nil
}

type fakeTx struct{ calls int }

func (f *fakeTx) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	f.calls++
	return fn(ctx)
}

type fixture struct {
	svc        *app.Service
	repo       *fakeRepo
	publisher  *fakePublisher
	tx         *fakeTx
	customerID string
	now        time.Time
}

func newFixture() *fixture {
	f := &fixture{
		repo:       &fakeRepo{orders: map[string]domain.Order{}},
		publisher:  &fakePublisher{},
		tx:         &fakeTx{},
		customerID: id.New(),
		now:        time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
	}
	f.svc = app.NewService(f.repo, fakeCustomers{f.customerID: true}, f.publisher, f.tx, clock.Fixed(f.now))
	return f
}

func validCommand(customerID string) app.PlaceOrder {
	return app.PlaceOrder{
		CustomerID: customerID,
		Currency:   "EUR",
		Lines: []app.PlaceOrderLine{
			{SKU: "BOOK-1", Quantity: 2, UnitPrice: 1250},
			{SKU: "PEN-7", Quantity: 1, UnitPrice: 499},
		},
	}
}

func TestPlaceStoresOrderAndPublishesEvent(t *testing.T) {
	f := newFixture()

	order, err := f.svc.Place(context.Background(), validCommand(f.customerID))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, stored := f.repo.orders[order.ID]; !stored {
		t.Fatal("order was not stored")
	}
	if f.tx.calls != 1 {
		t.Fatalf("expected exactly one transaction, got %d", f.tx.calls)
	}
	if len(f.publisher.published) != 1 {
		t.Fatalf("expected one event, got %d", len(f.publisher.published))
	}
	want := ordersapi.OrderPlaced{
		OrderID:     order.ID,
		CustomerID:  f.customerID,
		TotalAmount: 2999,
		Currency:    "EUR",
		PlacedAt:    f.now,
	}
	if got := f.publisher.published[0]; got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
}

func TestPlaceRejectsUnknownCustomer(t *testing.T) {
	f := newFixture()

	_, err := f.svc.Place(context.Background(), validCommand(id.New()))

	if !errors.Is(err, domain.ErrUnknownCustomer) {
		t.Fatalf("error = %v, want ErrUnknownCustomer", err)
	}
	if len(f.repo.orders) != 0 || len(f.publisher.published) != 0 {
		t.Fatal("nothing should be stored or published for a rejected order")
	}
}

func TestPlaceDoesNotPublishWhenStoringFails(t *testing.T) {
	f := newFixture()
	f.repo.insertErr = errors.New("database is down")

	_, err := f.svc.Place(context.Background(), validCommand(f.customerID))

	if !errors.Is(err, f.repo.insertErr) {
		t.Fatalf("error = %v, want the storage error", err)
	}
	if len(f.publisher.published) != 0 {
		t.Fatal("no event may be published when the order was not stored")
	}
}

func TestPlaceRejectsInvalidCurrency(t *testing.T) {
	f := newFixture()
	cmd := validCommand(f.customerID)
	cmd.Currency = "euros"

	if _, err := f.svc.Place(context.Background(), cmd); err == nil {
		t.Fatal("expected an error")
	}
}

func TestGetTreatsMalformedIDAsNotFound(t *testing.T) {
	f := newFixture()

	if _, err := f.svc.Get(context.Background(), "not-a-uuid"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}
