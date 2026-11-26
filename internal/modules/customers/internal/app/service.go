// Package app holds the customers module's use cases. It orchestrates the
// domain and talks to the outside world only through the ports declared
// here; adapters implement them.
package app

import (
	"context"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/id"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/customers/internal/domain"
)

// Repository is the persistence port for customers.
type Repository interface {
	// Insert stores a new customer. It returns domain.ErrEmailTaken when the
	// email is already in use.
	Insert(ctx context.Context, customer domain.Customer) error
	// ByID returns domain.ErrNotFound when no customer has the given ID.
	ByID(ctx context.Context, id string) (domain.Customer, error)
}

// Service implements the customers use cases.
type Service struct {
	repo  Repository
	clock clock.Clock
}

// NewService wires a Service.
func NewService(repo Repository, clk clock.Clock) *Service {
	return &Service{repo: repo, clock: clk}
}

// RegisterCustomer is the input of Service.Register.
type RegisterCustomer struct {
	Name  string
	Email string
}

// Register creates a new customer.
func (s *Service) Register(ctx context.Context, cmd RegisterCustomer) (domain.Customer, error) {
	customer, err := domain.NewCustomer(id.New(), cmd.Name, cmd.Email, s.clock.Now())
	if err != nil {
		return domain.Customer{}, err
	}
	if err := s.repo.Insert(ctx, customer); err != nil {
		return domain.Customer{}, err
	}
	return customer, nil
}

// Get returns a customer by ID.
func (s *Service) Get(ctx context.Context, customerID string) (domain.Customer, error) {
	if !id.Valid(customerID) {
		return domain.Customer{}, domain.ErrNotFound
	}
	return s.repo.ByID(ctx, customerID)
}
