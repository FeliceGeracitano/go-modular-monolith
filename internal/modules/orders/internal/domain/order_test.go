package domain_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/money"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/orders/internal/domain"
)

func eur(amount int64) money.Money { return money.Money{Amount: amount, Currency: "EUR"} }

func TestPlaceValidatesLines(t *testing.T) {
	tests := []struct {
		name    string
		lines   []domain.Line
		wantErr error
	}{
		{"valid", []domain.Line{{SKU: "A", Quantity: 1, UnitPrice: eur(100)}}, nil},
		{"free items are allowed", []domain.Line{{SKU: "GIFT", Quantity: 1, UnitPrice: eur(0)}}, nil},
		{"no lines", nil, domain.ErrNoLines},
		{"blank sku", []domain.Line{{SKU: " ", Quantity: 1, UnitPrice: eur(100)}}, domain.ErrInvalidLine},
		{"zero quantity", []domain.Line{{SKU: "A", Quantity: 0, UnitPrice: eur(100)}}, domain.ErrInvalidLine},
		{"negative price", []domain.Line{{SKU: "A", Quantity: 1, UnitPrice: eur(-1)}}, domain.ErrInvalidLine},
		{"total overflows", []domain.Line{{SKU: "A", Quantity: math.MaxInt64, UnitPrice: eur(2)}}, domain.ErrTotalTooLarge},
		{"mixed currencies", []domain.Line{
			{SKU: "A", Quantity: 1, UnitPrice: eur(100)},
			{SKU: "B", Quantity: 1, UnitPrice: money.Money{Amount: 100, Currency: "USD"}},
		}, domain.ErrMixedCurrencies},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.Place("order-1", "customer-1", tt.lines, time.Now())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestTotal(t *testing.T) {
	order, err := domain.Place("order-1", "customer-1", []domain.Line{
		{SKU: "A", Quantity: 2, UnitPrice: eur(1250)},
		{SKU: "B", Quantity: 1, UnitPrice: eur(499)},
	}, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := order.Total(); got != eur(2999) {
		t.Fatalf("Total() = %+v, want 2999 EUR", got)
	}
	if order.Status != domain.StatusPlaced {
		t.Fatalf("Status = %q, want %q", order.Status, domain.StatusPlaced)
	}
}
