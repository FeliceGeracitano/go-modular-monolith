package productsapi

import "context"

type Product struct {
	SKU      string
	Name     string
	Price    int64 // minor units
	Currency string
	Stock    int64
}

type API interface {
	GetBySKU(ctx context.Context, sku string) (Product, error)
}
