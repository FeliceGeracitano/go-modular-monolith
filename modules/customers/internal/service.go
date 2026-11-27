package core

import (
	"context"
	"slices"
	"strings"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/id"
)

type Repository interface {
	Insert(ctx context.Context, customer Customer) error
	ByID(ctx context.Context, id string) (Customer, error)
	List(ctx context.Context, limit, offset int) ([]Customer, int, error)
	Update(ctx context.Context, customer Customer) error
	Delete(ctx context.Context, id string) error
}

type Service struct {
	repo  Repository
	clock clock.Clock
}

func NewService(repo Repository, clk clock.Clock) *Service {
	return &Service{repo: repo, clock: clk}
}

type RegisterCustomer struct {
	Name  string
	Email string
}

type UpdateCustomer struct {
	Name  *string
	Email *string
}

type ListResult struct {
	Items  []Customer
	Total  int
	Limit  int
	Offset int
}

func (s *Service) Register(ctx context.Context, cmd RegisterCustomer) (Customer, error) {
	customer, err := NewCustomer(id.New(), cmd.Name, cmd.Email, s.clock.Now())
	if err != nil {
		return Customer{}, err
	}
	if err := s.repo.Insert(ctx, customer); err != nil {
		return Customer{}, err
	}
	return customer, nil
}

func (s *Service) Get(ctx context.Context, customerID string) (Customer, error) {
	if !id.Valid(customerID) {
		return Customer{}, ErrNotFound
	}
	return s.repo.ByID(ctx, customerID)
}

func (s *Service) List(ctx context.Context, limit, offset int) (ListResult, error) {
	limit, offset = clampPage(limit, offset)
	items, total, err := s.repo.List(ctx, limit, offset)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) Update(ctx context.Context, customerID string, cmd UpdateCustomer) (Customer, error) {
	if !id.Valid(customerID) {
		return Customer{}, ErrNotFound
	}
	existing, err := s.repo.ByID(ctx, customerID)
	if err != nil {
		return Customer{}, err
	}
	name, email := existing.Name, existing.Email
	if cmd.Name != nil {
		name = *cmd.Name
	}
	if cmd.Email != nil {
		email = *cmd.Email
	}
	updated, err := NewCustomer(existing.ID, name, email, existing.CreatedAt)
	if err != nil {
		return Customer{}, err
	}
	if err := s.repo.Update(ctx, updated); err != nil {
		return Customer{}, err
	}
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, customerID string) error {
	if !id.Valid(customerID) {
		return ErrNotFound
	}
	return s.repo.Delete(ctx, customerID)
}

func clampPage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func sortCustomers(items []Customer) {
	slices.SortFunc(items, func(a, b Customer) int {
		if cmp := a.CreatedAt.Compare(b.CreatedAt); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ID, b.ID)
	})
}
