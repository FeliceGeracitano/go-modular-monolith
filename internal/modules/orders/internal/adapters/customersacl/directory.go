// Package customersacl is the orders module's anti-corruption layer towards
// the customers module. It implements the app.CustomerDirectory port on top
// of the customers public API, so nothing else in orders knows that API
// exists.
package customersacl

import (
	"context"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/apperr"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/customers/customersapi"
)

// Directory implements app.CustomerDirectory.
type Directory struct {
	customers customersapi.API
}

// NewDirectory returns a Directory backed by the customers API.
func NewDirectory(customers customersapi.API) *Directory {
	return &Directory{customers: customers}
}

// Exists implements app.CustomerDirectory.
func (d *Directory) Exists(ctx context.Context, customerID string) (bool, error) {
	_, err := d.customers.GetCustomer(ctx, customerID)
	if apperr.KindOf(err) == apperr.KindNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
