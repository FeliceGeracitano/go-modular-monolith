// Package app holds the orders module's use cases. It orchestrates the
// domain and talks to the outside world only through the ports declared
// here; adapters implement them.
package app

import (
	"context"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/event"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/id"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/money"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders/internal/domain"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders/ordersapi"
)

// Repository is the persistence port for orders.
type Repository interface {
	Insert(ctx context.Context, order domain.Order) error
	// ByID returns domain.ErrNotFound when no order has the given ID.
	ByID(ctx context.Context, id string) (domain.Order, error)
}

// CustomerDirectory is what the orders module needs to know about
// customers. The port is phrased in this module's terms; an adapter
// translates it to the customers module's public API.
type CustomerDirectory interface {
	Exists(ctx context.Context, customerID string) (bool, error)
}

// EventPublisher records integration events. Implementations must make
// publication atomic with the surrounding transaction.
type EventPublisher interface {
	Publish(ctx context.Context, events ...event.Event) error
}

// Transactor runs a function inside a transaction that is propagated
// through the context to every repository and publisher call.
type Transactor interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Service implements the orders use cases.
type Service struct {
	repo      Repository
	customers CustomerDirectory
	events    EventPublisher
	tx        Transactor
	clock     clock.Clock
}

// NewService wires a Service.
func NewService(repo Repository, customers CustomerDirectory, events EventPublisher, tx Transactor, clk clock.Clock) *Service {
	return &Service{repo: repo, customers: customers, events: events, tx: tx, clock: clk}
}

// PlaceOrder is the input of Service.Place.
type PlaceOrder struct {
	CustomerID string
	Currency   string
	Lines      []PlaceOrderLine
}

// PlaceOrderLine is one line of a PlaceOrder command.
type PlaceOrderLine struct {
	SKU       string
	Quantity  int64
	UnitPrice int64 // minor units
}

// Place validates and stores a new order and announces it to the rest of
// the system.
func (s *Service) Place(ctx context.Context, cmd PlaceOrder) (domain.Order, error) {
	lines := make([]domain.Line, 0, len(cmd.Lines))
	for _, l := range cmd.Lines {
		price, err := money.New(l.UnitPrice, cmd.Currency)
		if err != nil {
			return domain.Order{}, err
		}
		lines = append(lines, domain.Line{SKU: l.SKU, Quantity: l.Quantity, UnitPrice: price})
	}

	order, err := domain.Place(id.New(), cmd.CustomerID, lines, s.clock.Now())
	if err != nil {
		return domain.Order{}, err
	}

	// Cross-module rule, checked through a port. This is a read from another
	// module, done before (not inside) our transaction.
	exists, err := s.customers.Exists(ctx, order.CustomerID)
	if err != nil {
		return domain.Order{}, err
	}
	if !exists {
		return domain.Order{}, domain.ErrUnknownCustomer
	}

	// State change and event are committed together or not at all.
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Insert(ctx, order); err != nil {
			return err
		}
		total := order.Total()
		return s.events.Publish(ctx, ordersapi.OrderPlaced{
			OrderID:     order.ID,
			CustomerID:  order.CustomerID,
			TotalAmount: total.Amount,
			Currency:    total.Currency,
			PlacedAt:    order.PlacedAt,
		})
	})
	if err != nil {
		return domain.Order{}, err
	}
	return order, nil
}

// Get returns an order by ID.
func (s *Service) Get(ctx context.Context, orderID string) (domain.Order, error) {
	if !id.Valid(orderID) {
		return domain.Order{}, domain.ErrNotFound
	}
	return s.repo.ByID(ctx, orderID)
}
