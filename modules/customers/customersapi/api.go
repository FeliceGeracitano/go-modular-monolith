package customersapi

import "context"

type Customer struct {
	ID    string
	Name  string
	Email string
}

type API interface {
	GetCustomer(ctx context.Context, id string) (Customer, error)
}
