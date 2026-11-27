package core

import (
	"context"
	"slices"
	"sync"
)

type MemoryStore struct {
	mu     sync.RWMutex
	orders map[string]Order
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{orders: make(map[string]Order)}
}

func (s *MemoryStore) Insert(_ context.Context, order Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	order.Lines = slices.Clone(order.Lines)
	s.orders[order.ID] = order
	return nil
}

func (s *MemoryStore) ByID(_ context.Context, id string) (Order, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	order, ok := s.orders[id]
	if !ok {
		return Order{}, ErrNotFound
	}
	order.Lines = slices.Clone(order.Lines)
	return order, nil
}

func (s *MemoryStore) List(_ context.Context, filter ListFilter) ([]Order, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var matched []Order
	for _, o := range s.orders {
		if filter.CustomerID != "" && o.CustomerID != filter.CustomerID {
			continue
		}
		if filter.Status != "" && o.Status != filter.Status {
			continue
		}
		o.Lines = slices.Clone(o.Lines)
		matched = append(matched, o)
	}
	sortOrders(matched)
	total := len(matched)
	if filter.Offset >= total {
		return []Order{}, total, nil
	}
	end := min(filter.Offset+filter.Limit, total)
	return matched[filter.Offset:end], total, nil
}

func (s *MemoryStore) Update(_ context.Context, order Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orders[order.ID]; !ok {
		return ErrNotFound
	}
	order.Lines = slices.Clone(order.Lines)
	s.orders[order.ID] = order
	return nil
}
