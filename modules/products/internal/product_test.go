package core_test

import (
	"errors"
	"testing"
	"time"

	core "github.com/FeliceGeracitano/go-modular-monolith/modules/products/internal"
)

func TestNewProduct(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		sku     string
		pName   string
		price   int64
		curr    string
		stock   int64
		wantErr error
	}{
		{"valid", "book-1", "Go Book", 2500, "eur", 10, nil},
		{"blank sku", " ", "Go Book", 2500, "EUR", 10, core.ErrInvalidSKU},
		{"blank name", "BOOK-1", " ", 2500, "EUR", 10, core.ErrInvalidName},
		{"negative price", "BOOK-1", "Go Book", -1, "EUR", 10, core.ErrInvalidPrice},
		{"bad currency", "BOOK-1", "Go Book", 2500, "EU", 10, core.ErrInvalidCurrency},
		{"negative stock", "BOOK-1", "Go Book", 2500, "EUR", -1, core.ErrInvalidStock},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := core.NewProduct(tt.sku, tt.pName, tt.price, tt.curr, tt.stock, now)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err == nil && (p.SKU != "BOOK-1" || p.Currency != "EUR") {
				t.Fatalf("unexpected normalised product: %+v", p)
			}
		})
	}
}
