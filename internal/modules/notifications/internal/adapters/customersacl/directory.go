// Package customersacl is the notifications module's anti-corruption layer
// towards the customers module.
package customersacl

import (
	"context"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/customers/customersapi"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/notifications/internal/app"
)

// Directory implements app.CustomerDirectory.
type Directory struct {
	customers customersapi.API
}

// NewDirectory returns a Directory backed by the customers API.
func NewDirectory(customers customersapi.API) *Directory {
	return &Directory{customers: customers}
}

// Contact implements app.CustomerDirectory.
func (d *Directory) Contact(ctx context.Context, customerID string) (app.Contact, error) {
	c, err := d.customers.GetCustomer(ctx, customerID)
	if err != nil {
		return app.Contact{}, err
	}
	return app.Contact{Name: c.Name, Email: c.Email}, nil
}
