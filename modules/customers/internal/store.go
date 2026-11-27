package core

import (
	"context"
	"sync"
)

type MemoryStore struct {
	mu      sync.RWMutex
	byID    map[string]Customer
	byEmail map[string]string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		byID:    make(map[string]Customer),
		byEmail: make(map[string]string),
	}
}

func (s *MemoryStore) Insert(_ context.Context, customer Customer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byEmail[customer.Email]; exists {
		return ErrEmailTaken
	}
	s.byID[customer.ID] = customer
	s.byEmail[customer.Email] = customer.ID
	return nil
}

func (s *MemoryStore) ByID(_ context.Context, id string) (Customer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	customer, ok := s.byID[id]
	if !ok {
		return Customer{}, ErrNotFound
	}
	return customer, nil
}

func (s *MemoryStore) List(_ context.Context, limit, offset int) ([]Customer, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all := make([]Customer, 0, len(s.byID))
	for _, c := range s.byID {
		all = append(all, c)
	}
	sortCustomers(all)
	total := len(all)
	if offset >= total {
		return []Customer{}, total, nil
	}
	end := min(offset+limit, total)
	return all[offset:end], total, nil
}

func (s *MemoryStore) Update(_ context.Context, customer Customer) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.byID[customer.ID]
	if !ok {
		return ErrNotFound
	}
	if customer.Email != existing.Email {
		if ownerID, exists := s.byEmail[customer.Email]; exists && ownerID != customer.ID {
			return ErrEmailTaken
		}
		delete(s.byEmail, existing.Email)
		s.byEmail[customer.Email] = customer.ID
	}
	s.byID[customer.ID] = customer
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.byID[id]
	if !ok {
		return ErrNotFound
	}
	delete(s.byID, id)
	delete(s.byEmail, existing.Email)
	return nil
}
