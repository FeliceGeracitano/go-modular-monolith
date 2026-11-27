package core

import (
	"context"
	"log/slog"
	"sync"

	"github.com/FeliceGeracitano/go-modular-monolith/modules/customers/customersapi"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/orders/ordersapi"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/eventbus"
)

type MemoryStore struct {
	mu      sync.RWMutex
	byEvent map[string]Notification
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{byEvent: make(map[string]Notification)}
}

func (s *MemoryStore) Exists(_ context.Context, eventID string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.byEvent[eventID]
	return ok, nil
}

func (s *MemoryStore) Save(_ context.Context, n Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byEvent[n.EventID] = n
	return nil
}

func (s *MemoryStore) List(_ context.Context, customerID string, limit, offset int) ([]Notification, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var matched []Notification
	for _, n := range s.byEvent {
		if customerID != "" && n.CustomerID != customerID {
			continue
		}
		matched = append(matched, n)
	}
	sortNotifications(matched)
	total := len(matched)
	if offset >= total {
		return []Notification{}, total, nil
	}
	end := min(offset+limit, total)
	return matched[offset:end], total, nil
}

type CustomersACL struct {
	customers customersapi.API
}

func NewCustomerDirectory(customers customersapi.API) *CustomersACL {
	return &CustomersACL{customers: customers}
}

func (d *CustomersACL) Contact(ctx context.Context, customerID string) (Contact, error) {
	c, err := d.customers.GetCustomer(ctx, customerID)
	if err != nil {
		return Contact{}, err
	}
	return Contact{Name: c.Name, Email: c.Email}, nil
}

type LogSender struct {
	logger *slog.Logger
}

func NewLogSender(logger *slog.Logger) *LogSender {
	return &LogSender{logger: logger}
}

func (s *LogSender) Send(ctx context.Context, n Notification) error {
	s.logger.InfoContext(ctx, "notification sent",
		"notification_id", n.ID, "event_id", n.EventID, "recipient", n.Recipient, "subject", n.Subject)
	return nil
}

func RegisterSubscribers(sub eventbus.Subscriber, svc *Service) {
	sub.Subscribe(ordersapi.OrderPlacedName, "notifications.order_placed",
		eventbus.Typed(func(ctx context.Context, env eventbus.Envelope, evt ordersapi.OrderPlaced) error {
			return svc.NotifyOrderPlaced(ctx, OrderPlaced{
				EventID:     env.ID,
				OrderID:     evt.OrderID,
				CustomerID:  evt.CustomerID,
				TotalAmount: evt.TotalAmount,
				Currency:    evt.Currency,
			})
		}))

	sub.Subscribe(ordersapi.OrderCancelledName, "notifications.order_cancelled",
		eventbus.Typed(func(ctx context.Context, env eventbus.Envelope, evt ordersapi.OrderCancelled) error {
			return svc.NotifyOrderCancelled(ctx, OrderCancelled{
				EventID:    env.ID,
				OrderID:    evt.OrderID,
				CustomerID: evt.CustomerID,
			})
		}))
}
