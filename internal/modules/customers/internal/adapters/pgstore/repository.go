// Package pgstore is the PostgreSQL adapter for the customers module. It
// only ever touches tables in the "customers" schema.
package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/customers/internal/domain"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres"
)

// Repository implements app.Repository.
type Repository struct {
	db *postgres.DB
}

// NewRepository returns a Repository backed by db.
func NewRepository(db *postgres.DB) *Repository {
	return &Repository{db: db}
}

// Insert implements app.Repository.
func (r *Repository) Insert(ctx context.Context, c domain.Customer) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`INSERT INTO customers.customers (id, name, email, created_at) VALUES ($1, $2, $3, $4)`,
		c.ID, c.Name, c.Email, c.CreatedAt)
	if postgres.IsUniqueViolation(err, "customers_email_key") {
		return domain.ErrEmailTaken
	}
	if err != nil {
		return fmt.Errorf("customers: insert: %w", err)
	}
	return nil
}

// ByID implements app.Repository.
func (r *Repository) ByID(ctx context.Context, id string) (domain.Customer, error) {
	var c domain.Customer
	err := r.db.Conn(ctx).QueryRow(ctx,
		`SELECT id::text, name, email, created_at FROM customers.customers WHERE id = $1`, id).
		Scan(&c.ID, &c.Name, &c.Email, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Customer{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Customer{}, fmt.Errorf("customers: select by id: %w", err)
	}
	return c, nil
}
