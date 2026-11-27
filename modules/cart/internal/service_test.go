package core_test

import (
	"context"
	"errors"
	"testing"

	core "github.com/FeliceGeracitano/go-modular-monolith/modules/cart/internal"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/id"
)

type fakeCustomers map[string]bool

func (f fakeCustomers) Exists(_ context.Context, cid string) (bool, error) { return f[cid], nil }

type fakeCatalog map[string]core.ProductInfo

func (f fakeCatalog) GetBySKU(_ context.Context, sku string) (core.ProductInfo, bool, error) {
	p, ok := f[sku]
	return p, ok, nil
}

func TestCartAddRemoveAndStockLimits(t *testing.T) {
	cid := id.New()
	svc := core.NewService(
		core.NewMemoryStore(),
		fakeCustomers{cid: true},
		fakeCatalog{
			"BOOK-1": {SKU: "BOOK-1", Name: "Go Book", Price: 2500, Currency: "EUR", Stock: 3},
			"USD-1":  {SKU: "USD-1", Name: "US Item", Price: 1000, Currency: "USD", Stock: 5},
		},
	)
	ctx := context.Background()

	c, err := svc.AddItem(ctx, cid, "book-1", 2)
	if err != nil || c.TotalAmount() != 5000 || len(c.Items) != 1 {
		t.Fatalf("AddItem = %+v, err=%v", c, err)
	}
	if _, err := svc.AddItem(ctx, cid, "BOOK-1", 2); !errors.Is(err, core.ErrInsufficientStock) {
		t.Fatalf("exceeding stock err = %v, want ErrInsufficientStock", err)
	}
	if _, err := svc.AddItem(ctx, cid, "USD-1", 1); !errors.Is(err, core.ErrMixedCurrencies) {
		t.Fatalf("mixed currency err = %v, want ErrMixedCurrencies", err)
	}
	c, err = svc.RemoveItem(ctx, cid, "BOOK-1")
	if err != nil || len(c.Items) != 0 || c.Currency != "" {
		t.Fatalf("RemoveItem = %+v, err=%v", c, err)
	}
}
