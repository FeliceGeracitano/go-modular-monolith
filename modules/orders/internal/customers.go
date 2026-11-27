package core

import (
	"context"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/apperr"
	"github.com/FeliceGeracitano/go-modular-monolith/modules/customers/customersapi"
)

type CustomersACL struct {
	customers customersapi.API
}

func NewCustomerDirectory(customers customersapi.API) *CustomersACL {
	return &CustomersACL{customers: customers}
}

func (d *CustomersACL) Exists(ctx context.Context, customerID string) (bool, error) {
	_, err := d.customers.GetCustomer(ctx, customerID)
	if apperr.KindOf(err) == apperr.KindNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
