package core

import (
	"strings"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/apperr"
)

var (
	ErrNotFound         = apperr.NotFound("orders.not_found", "order not found")
	ErrUnknownCustomer  = apperr.Invalid("orders.unknown_customer", "customer does not exist")
	ErrNoLines          = apperr.Invalid("orders.no_lines", "an order needs at least one line")
	ErrInvalidLine      = apperr.Invalid("orders.invalid_line", "each line needs a SKU, a positive quantity and a non-negative unit price")
	ErrMixedCurrencies  = apperr.Invalid("orders.mixed_currencies", "all lines must use the same currency")
	ErrTotalTooLarge    = apperr.Invalid("orders.total_too_large", "order total is too large")
	ErrAlreadyCancelled = apperr.Conflict("orders.already_cancelled", "order is already cancelled")
)

type Status string

const (
	StatusPlaced    Status = "placed"
	StatusCancelled Status = "cancelled"
)

type Line struct {
	SKU       string
	Quantity  int64
	UnitPrice Money
}

type Order struct {
	ID          string
	CustomerID  string
	Lines       []Line
	Status      Status
	PlacedAt    time.Time
	CancelledAt *time.Time
}

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

func (o *Order) Cancel(at time.Time) error {
	if o.Status == StatusCancelled {
		return ErrAlreadyCancelled
	}
	t := at
	o.Status = StatusCancelled
	o.CancelledAt = &t
	return nil
}

func (o Order) Total() Money {
	total, err := o.total()
	if err != nil {
		panic("orders: invalid order reached Total: " + err.Error())
	}
	return total
}

func (o Order) total() (Money, error) {
	total := Money{Currency: o.Lines[0].UnitPrice.Currency}
	for _, line := range o.Lines {
		lineTotal, err := line.UnitPrice.Mul(line.Quantity)
		if err != nil {
			return Money{}, err
		}
		if total, err = total.Add(lineTotal); err != nil {
			return Money{}, err
		}
	}
	return total, nil
}
