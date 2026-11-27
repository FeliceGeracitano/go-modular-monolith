package core

import (
	"strings"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/apperr"
)

var (
	ErrNotFound        = apperr.NotFound("products.not_found", "product not found")
	ErrSKUTaken        = apperr.Conflict("products.sku_taken", "a product with this SKU already exists")
	ErrInvalidSKU      = apperr.Invalid("products.invalid_sku", "sku must be 1 to 64 non-whitespace characters")
	ErrInvalidName     = apperr.Invalid("products.invalid_name", "name must be 1 to 200 characters")
	ErrInvalidPrice    = apperr.Invalid("products.invalid_price", "price must be non-negative")
	ErrInvalidCurrency = apperr.Invalid("products.invalid_currency", "currency must be a 3-letter ISO 4217 code")
	ErrInvalidStock    = apperr.Invalid("products.invalid_stock", "stock must be non-negative")
)

type Product struct {
	SKU       string
	Name      string
	Price     int64 // minor units
	Currency  string
	Stock     int64
	CreatedAt time.Time
}

func NewProduct(sku, name string, price int64, currency string, stock int64, now time.Time) (Product, error) {
	sku = strings.ToUpper(strings.TrimSpace(sku))
	if sku == "" || len(sku) > 64 || strings.ContainsAny(sku, " \t\n\r/") {
		return Product{}, ErrInvalidSKU
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return Product{}, ErrInvalidName
	}
	if price < 0 {
		return Product{}, ErrInvalidPrice
	}
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if !validCurrency(currency) {
		return Product{}, ErrInvalidCurrency
	}
	if stock < 0 {
		return Product{}, ErrInvalidStock
	}
	return Product{
		SKU:       sku,
		Name:      name,
		Price:     price,
		Currency:  currency,
		Stock:     stock,
		CreatedAt: now,
	}, nil
}

func validCurrency(code string) bool {
	if len(code) != 3 {
		return false
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
