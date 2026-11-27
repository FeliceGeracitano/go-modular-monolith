package core_test

import (
	"errors"
	"math"
	"testing"
	"time"

	core "github.com/FeliceGeracitano/go-modular-monolith/modules/orders/internal"
)

func eur(amount int64) core.Money { return core.Money{Amount: amount, Currency: "EUR"} }

func TestPlaceValidatesLines(t *testing.T) {
	tests := []struct {
		name    string
		lines   []core.Line
		wantErr error
	}{
		{"valid", []core.Line{{SKU: "A", Quantity: 1, UnitPrice: eur(100)}}, nil},
		{"free items", []core.Line{{SKU: "GIFT", Quantity: 1, UnitPrice: eur(0)}}, nil},
		{"no lines", nil, core.ErrNoLines},
		{"blank sku", []core.Line{{SKU: " ", Quantity: 1, UnitPrice: eur(100)}}, core.ErrInvalidLine},
		{"zero quantity", []core.Line{{SKU: "A", Quantity: 0, UnitPrice: eur(100)}}, core.ErrInvalidLine},
		{"negative price", []core.Line{{SKU: "A", Quantity: 1, UnitPrice: eur(-1)}}, core.ErrInvalidLine},
		{"total overflows", []core.Line{{SKU: "A", Quantity: math.MaxInt64, UnitPrice: eur(2)}}, core.ErrTotalTooLarge},
		{"mixed currencies", []core.Line{
			{SKU: "A", Quantity: 1, UnitPrice: eur(100)},
			{SKU: "B", Quantity: 1, UnitPrice: core.Money{Amount: 100, Currency: "USD"}},
		}, core.ErrMixedCurrencies},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := core.Place("order-1", "customer-1", tt.lines, time.Now())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestTotal(t *testing.T) {
	order, err := core.Place("order-1", "customer-1", []core.Line{
		{SKU: "A", Quantity: 2, UnitPrice: eur(1250)},
		{SKU: "B", Quantity: 1, UnitPrice: eur(499)},
	}, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := order.Total(); got != eur(2999) {
		t.Fatalf("Total() = %+v, want 2999 EUR", got)
	}
	if order.Status != core.StatusPlaced {
		t.Fatalf("Status = %q, want %q", order.Status, core.StatusPlaced)
	}
}
