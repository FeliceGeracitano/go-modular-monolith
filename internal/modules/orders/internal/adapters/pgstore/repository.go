// Package pgstore is the PostgreSQL adapter for the orders module. It only
// ever touches tables in the "orders" schema.
package pgstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/money"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders/internal/domain"
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

// Insert implements app.Repository. The order and its lines are written in
// one transaction: it joins the caller's, or starts its own.
func (r *Repository) Insert(ctx context.Context, o domain.Order) error {
	return r.db.WithinTx(ctx, func(ctx context.Context) error {
		conn := r.db.Conn(ctx)
		total := o.Total()

		_, err := conn.Exec(ctx, `
			INSERT INTO orders.orders (id, customer_id, status, currency, total_amount, placed_at)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			o.ID, o.CustomerID, string(o.Status), total.Currency, total.Amount, o.PlacedAt)
		if err != nil {
			return fmt.Errorf("orders: insert order: %w", err)
		}

		for i, line := range o.Lines {
			_, err := conn.Exec(ctx, `
				INSERT INTO orders.order_lines (order_id, line_no, sku, quantity, unit_price)
				VALUES ($1, $2, $3, $4, $5)`,
				o.ID, i+1, line.SKU, line.Quantity, line.UnitPrice.Amount)
			if err != nil {
				return fmt.Errorf("orders: insert line %d: %w", i+1, err)
			}
		}
		return nil
	})
}

// ByID implements app.Repository.
func (r *Repository) ByID(ctx context.Context, id string) (domain.Order, error) {
	conn := r.db.Conn(ctx)

	var (
		o        domain.Order
		status   string
		currency string
	)
	err := conn.QueryRow(ctx,
		`SELECT id::text, customer_id::text, status, currency, placed_at FROM orders.orders WHERE id = $1`, id).
		Scan(&o.ID, &o.CustomerID, &status, &currency, &o.PlacedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("orders: select order: %w", err)
	}
	o.Status = domain.Status(status)

	rows, err := conn.Query(ctx,
		`SELECT sku, quantity, unit_price FROM orders.order_lines WHERE order_id = $1 ORDER BY line_no`, id)
	if err != nil {
		return domain.Order{}, fmt.Errorf("orders: select lines: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		line := domain.Line{UnitPrice: money.Money{Currency: currency}}
		if err := rows.Scan(&line.SKU, &line.Quantity, &line.UnitPrice.Amount); err != nil {
			return domain.Order{}, fmt.Errorf("orders: scan line: %w", err)
		}
		o.Lines = append(o.Lines, line)
	}
	if err := rows.Err(); err != nil {
		return domain.Order{}, fmt.Errorf("orders: read lines: %w", err)
	}
	return o, nil
}
