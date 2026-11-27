package core

import (
	"context"
	"slices"
	"strings"

	"github.com/FeliceGeracitano/go-modular-monolith/modules/orders/ordersapi"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/event"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/id"
)

type ListFilter struct {
	CustomerID string
	Status     Status
	Limit      int
	Offset     int
}

type ListResult struct {
	Items  []Order
	Total  int
	Limit  int
	Offset int
}

type Repository interface {
	Insert(ctx context.Context, order Order) error
	ByID(ctx context.Context, id string) (Order, error)
	List(ctx context.Context, filter ListFilter) ([]Order, int, error)
	Update(ctx context.Context, order Order) error
}

type CustomerDirectory interface {
	Exists(ctx context.Context, customerID string) (bool, error)
}

type EventPublisher interface {
	Publish(ctx context.Context, events ...event.Event) error
}

type Service struct {
	repo      Repository
	customers CustomerDirectory
	events    EventPublisher
	clock     clock.Clock
}

func NewService(repo Repository, customers CustomerDirectory, events EventPublisher, clk clock.Clock) *Service {
	return &Service{repo: repo, customers: customers, events: events, clock: clk}
}

type PlaceOrder struct {
	CustomerID string
	Currency   string
	Lines      []PlaceOrderLine
}

type PlaceOrderLine struct {
	SKU       string
	Quantity  int64
	UnitPrice int64 // minor units
}

func (s *Service) Place(ctx context.Context, cmd PlaceOrder) (Order, error) {
	lines := make([]Line, 0, len(cmd.Lines))
	for _, l := range cmd.Lines {
		price, err := NewMoney(l.UnitPrice, cmd.Currency)
		if err != nil {
			return Order{}, err
		}
		lines = append(lines, Line{SKU: l.SKU, Quantity: l.Quantity, UnitPrice: price})
	}

	order, err := Place(id.New(), cmd.CustomerID, lines, s.clock.Now())
	if err != nil {
		return Order{}, err
	}

	exists, err := s.customers.Exists(ctx, order.CustomerID)
	if err != nil {
		return Order{}, err
	}
	if !exists {
		return Order{}, ErrUnknownCustomer
	}

	if err := s.repo.Insert(ctx, order); err != nil {
		return Order{}, err
	}

	total := order.Total()
	if err := s.events.Publish(ctx, ordersapi.OrderPlaced{
		OrderID:     order.ID,
		CustomerID:  order.CustomerID,
		TotalAmount: total.Amount,
		Currency:    total.Currency,
		PlacedAt:    order.PlacedAt,
	}); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (s *Service) Get(ctx context.Context, orderID string) (Order, error) {
	if !id.Valid(orderID) {
		return Order{}, ErrNotFound
	}
	return s.repo.ByID(ctx, orderID)
}

func (s *Service) List(ctx context.Context, filter ListFilter) (ListResult, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, Total: total, Limit: filter.Limit, Offset: filter.Offset}, nil
}

func (s *Service) Cancel(ctx context.Context, orderID string) (Order, error) {
	if !id.Valid(orderID) {
		return Order{}, ErrNotFound
	}
	order, err := s.repo.ByID(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	now := s.clock.Now()
	if err := order.Cancel(now); err != nil {
		return Order{}, err
	}
	if err := s.repo.Update(ctx, order); err != nil {
		return Order{}, err
	}
	if err := s.events.Publish(ctx, ordersapi.OrderCancelled{
		OrderID:     order.ID,
		CustomerID:  order.CustomerID,
		CancelledAt: now,
	}); err != nil {
		return Order{}, err
	}
	return order, nil
}

func sortOrders(items []Order) {
	slices.SortFunc(items, func(a, b Order) int {
		if cmp := a.PlacedAt.Compare(b.PlacedAt); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ID, b.ID)
	})
}
