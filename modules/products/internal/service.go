package core

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/clock"
)

type Repository interface {
	Insert(ctx context.Context, p Product) error
	BySKU(ctx context.Context, sku string) (Product, error)
	List(ctx context.Context, limit, offset int) ([]Product, int, error)
	Update(ctx context.Context, p Product) error
	Delete(ctx context.Context, sku string) error
}

type CreateProduct struct {
	SKU      string
	Name     string
	Price    int64
	Currency string
	Stock    int64
}

type UpdateProduct struct {
	Name  *string
	Price *int64
	Stock *int64
}

type ListResult struct {
	Items  []Product
	Total  int
	Limit  int
	Offset int
}

type Service struct {
	repo  Repository
	clock clock.Clock
}

func NewService(repo Repository, clk clock.Clock) *Service {
	return &Service{repo: repo, clock: clk}
}

func (s *Service) Create(ctx context.Context, cmd CreateProduct) (Product, error) {
	p, err := NewProduct(cmd.SKU, cmd.Name, cmd.Price, cmd.Currency, cmd.Stock, s.clock.Now())
	if err != nil {
		return Product{}, err
	}
	if err := s.repo.Insert(ctx, p); err != nil {
		return Product{}, err
	}
	return p, nil
}

func (s *Service) Get(ctx context.Context, sku string) (Product, error) {
	sku = strings.ToUpper(strings.TrimSpace(sku))
	if sku == "" {
		return Product{}, ErrNotFound
	}
	return s.repo.BySKU(ctx, sku)
}

func (s *Service) List(ctx context.Context, limit, offset int) (ListResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	items, total, err := s.repo.List(ctx, limit, offset)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) Update(ctx context.Context, sku string, cmd UpdateProduct) (Product, error) {
	existing, err := s.Get(ctx, sku)
	if err != nil {
		return Product{}, err
	}
	name, price, stock := existing.Name, existing.Price, existing.Stock
	if cmd.Name != nil {
		name = *cmd.Name
	}
	if cmd.Price != nil {
		price = *cmd.Price
	}
	if cmd.Stock != nil {
		stock = *cmd.Stock
	}
	updated, err := NewProduct(existing.SKU, name, price, existing.Currency, stock, existing.CreatedAt)
	if err != nil {
		return Product{}, err
	}
	if err := s.repo.Update(ctx, updated); err != nil {
		return Product{}, err
	}
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, sku string) error {
	sku = strings.ToUpper(strings.TrimSpace(sku))
	if sku == "" {
		return ErrNotFound
	}
	return s.repo.Delete(ctx, sku)
}

type MemoryStore struct {
	mu    sync.RWMutex
	bySKU map[string]Product
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{bySKU: make(map[string]Product)}
}

func (s *MemoryStore) Insert(_ context.Context, p Product) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.bySKU[p.SKU]; exists {
		return ErrSKUTaken
	}
	s.bySKU[p.SKU] = p
	return nil
}

func (s *MemoryStore) BySKU(_ context.Context, sku string) (Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.bySKU[sku]
	if !ok {
		return Product{}, ErrNotFound
	}
	return p, nil
}

func (s *MemoryStore) List(_ context.Context, limit, offset int) ([]Product, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := make([]Product, 0, len(s.bySKU))
	for _, p := range s.bySKU {
		all = append(all, p)
	}
	slices.SortFunc(all, func(a, b Product) int { return strings.Compare(a.SKU, b.SKU) })
	total := len(all)
	if offset >= total {
		return []Product{}, total, nil
	}
	end := min(offset+limit, total)
	return all[offset:end], total, nil
}

func (s *MemoryStore) Update(_ context.Context, p Product) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bySKU[p.SKU]; !ok {
		return ErrNotFound
	}
	s.bySKU[p.SKU] = p
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, sku string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bySKU[sku]; !ok {
		return ErrNotFound
	}
	delete(s.bySKU, sku)
	return nil
}
