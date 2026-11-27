package core_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/event"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/id"
	core "github.com/FeliceGeracitano/go-modular-monolith/modules/orders/internal"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/orders/ordersapi"
)

type fakeRepo struct {
	orders    map[string]core.Order
	insertErr error
}

func (f *fakeRepo) Insert(_ context.Context, o core.Order) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	f.orders[o.ID] = o
	return nil
}

func (f *fakeRepo) ByID(_ context.Context, id string) (core.Order, error) {
	o, ok := f.orders[id]
	if !ok {
		return core.Order{}, core.ErrNotFound
	}
	return o, nil
}

func (f *fakeRepo) List(_ context.Context, _ core.ListFilter) ([]core.Order, int, error) {
	out := make([]core.Order, 0, len(f.orders))
	for _, o := range f.orders {
		out = append(out, o)
	}
	return out, len(out), nil
}

func (f *fakeRepo) Update(_ context.Context, o core.Order) error {
	f.orders[o.ID] = o
	return nil
}

type fakeCustomers map[string]bool

func (f fakeCustomers) Exists(_ context.Context, id string) (bool, error) { return f[id], nil }

type fakePublisher struct{ published []event.Event }

func (f *fakePublisher) Publish(_ context.Context, events ...event.Event) error {
	f.published = append(f.published, events...)
	return nil
}

type fixture struct {
	svc        *core.Service
	repo       *fakeRepo
	publisher  *fakePublisher
	customerID string
	now        time.Time
}

func newFixture() *fixture {
	f := &fixture{
		repo:       &fakeRepo{orders: map[string]core.Order{}},
		publisher:  &fakePublisher{},
		customerID: id.New(),
		now:        time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC),
	}
	f.svc = core.NewService(f.repo, fakeCustomers{f.customerID: true}, f.publisher, clock.Fixed(f.now))
	return f
}

func validCommand(customerID string) core.PlaceOrder {
	return core.PlaceOrder{
		CustomerID: customerID,
		Currency:   "EUR",
		Lines: []core.PlaceOrderLine{
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
	if !errors.Is(err, core.ErrUnknownCustomer) {
		t.Fatalf("error = %v, want ErrUnknownCustomer", err)
	}
	if len(f.repo.orders) != 0 || len(f.publisher.published) != 0 {
		t.Fatal("nothing should be stored or published for a rejected order")
	}
}

func TestPlaceDoesNotPublishWhenStoringFails(t *testing.T) {
	f := newFixture()
	f.repo.insertErr = errors.New("storage unavailable")

	_, err := f.svc.Place(context.Background(), validCommand(f.customerID))
	if !errors.Is(err, f.repo.insertErr) {
		t.Fatalf("error = %v, want storage error", err)
	}
	if len(f.publisher.published) != 0 {
		t.Fatal("no event may be published when storing fails")
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

	if _, err := f.svc.Get(context.Background(), "not-a-uuid"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestCancelTransitionsOrderAndPublishesEvent(t *testing.T) {
	f := newFixture()

	placed, err := f.svc.Place(context.Background(), validCommand(f.customerID))
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := f.svc.Cancel(context.Background(), placed.ID)
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if cancelled.Status != core.StatusCancelled || cancelled.CancelledAt == nil {
		t.Fatalf("unexpected cancelled order: %+v", cancelled)
	}
	if len(f.publisher.published) != 2 {
		t.Fatalf("published %d events, want 2", len(f.publisher.published))
	}
	if _, ok := f.publisher.published[1].(ordersapi.OrderCancelled); !ok {
		t.Fatalf("second event = %T, want OrderCancelled", f.publisher.published[1])
	}
	if _, err := f.svc.Cancel(context.Background(), placed.ID); !errors.Is(err, core.ErrAlreadyCancelled) {
		t.Fatalf("second cancel error = %v, want ErrAlreadyCancelled", err)
	}
}
