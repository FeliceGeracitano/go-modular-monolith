// Package domain holds the orders module's business rules. It is pure: no
// I/O, no framework, only the standard library and the shared kernel.
package domain

import (
	"strings"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/apperr"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/money"
)

// Business errors of the orders module.
var (
	ErrNotFound        = apperr.NotFound("orders.not_found", "order not found")
	ErrUnknownCustomer = apperr.Invalid("orders.unknown_customer", "customer does not exist")
	ErrNoLines         = apperr.Invalid("orders.no_lines", "an order needs at least one line")
	ErrInvalidLine     = apperr.Invalid("orders.invalid_line", "each line needs a SKU, a positive quantity and a non-negative unit price")
	ErrMixedCurrencies = apperr.Invalid("orders.mixed_currencies", "all lines must use the same currency")
	ErrTotalTooLarge   = apperr.Invalid("orders.total_too_large", "order total is too large")
)

// Status is the lifecycle state of an order.
type Status string

// Order statuses.
const (
	StatusPlaced Status = "placed"
)

// Line is one product on an order.
type Line struct {
	SKU       string
	Quantity  int64
	UnitPrice money.Money
}

// Order is the aggregate root of the orders module.
type Order struct {
	ID         string
	CustomerID string
	Lines      []Line
	Status     Status
	PlacedAt   time.Time
}

// Place validates the lines and returns a new placed order. Whether the
// customer exists is not this function's concern: that requires another
// module and is checked by the application layer.
func Place(id, customerID string, lines []Line, now time.Time) (Order, error) {
	if len(lines) == 0 {
		return Order{}, ErrNoLines
	}
	currency := lines[0].UnitPrice.Currency
	for i := range lines {
		lines[i].SKU = strings.TrimSpace(lines[i].SKU)
		line := lines[i]
		if line.SKU == "" || line.Quantity <= 0 || line.UnitPrice.Amount < 0 {
			return Order{}, ErrInvalidLine
		}
		if line.UnitPrice.Currency != currency {
			return Order{}, ErrMixedCurrencies
		}
	}
	order := Order{
		ID:         id,
		CustomerID: customerID,
		Lines:      lines,
		Status:     StatusPlaced,
		PlacedAt:   now,
	}
	if _, err := order.total(); err != nil {
		return Order{}, ErrTotalTooLarge
	}
	return order, nil
}

// Total is the sum of every line. Place guarantees an order has at least
// one line, a single currency and a total that fits, so it cannot fail.
func (o Order) Total() money.Money {
	total, err := o.total()
	if err != nil {
		panic("orders: invalid order reached Total: " + err.Error()) // unreachable for orders built by Place
	}
	return total
}

func (o Order) total() (money.Money, error) {
	total := money.Money{Currency: o.Lines[0].UnitPrice.Currency}
	for _, line := range o.Lines {
		lineTotal, err := line.UnitPrice.Mul(line.Quantity)
		if err != nil {
			return money.Money{}, err
		}
		if total, err = total.Add(lineTotal); err != nil {
			return money.Money{}, err
		}
	}
	return total, nil
}
