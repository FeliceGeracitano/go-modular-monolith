// Package customersapi is the customers module's public contract: the only
// part of the module other modules are allowed to import.
//
// It contains plain data and interfaces, no behaviour, and depends on
// nothing but the standard library and the shared kernel. Treat every change
// here as a change to a published API.
package customersapi

import "context"

// Customer is the view of a customer shared with other modules. It is a DTO,
// deliberately decoupled from the module's internal domain model so that the
// model can change without breaking consumers.
type Customer struct {
	ID    string
	Name  string
	Email string
}

// API is the synchronous interface other modules use to query customers.
type API interface {
	// GetCustomer returns the customer with the given ID. A missing customer
	// is reported as an error of kind apperr.KindNotFound.
	GetCustomer(ctx context.Context, id string) (Customer, error)
}
