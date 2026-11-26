// Package domain holds the customers module's business rules. It is pure:
// no I/O, no framework, only the standard library and the shared kernel.
package domain

import (
	"net/mail"
	"strings"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/apperr"
)

// Business errors of the customers module.
var (
	ErrNotFound     = apperr.NotFound("customers.not_found", "customer not found")
	ErrEmailTaken   = apperr.Conflict("customers.email_taken", "a customer with this email already exists")
	ErrInvalidName  = apperr.Invalid("customers.invalid_name", "name must be between 1 and 200 characters")
	ErrInvalidEmail = apperr.Invalid("customers.invalid_email", "email must be a valid address")
)

// Customer is a person or company that can place orders.
type Customer struct {
	ID        string
	Name      string
	Email     string
	CreatedAt time.Time
}

// NewCustomer validates and normalises its input and returns a Customer that
// satisfies every invariant.
func NewCustomer(id, name, email string, now time.Time) (Customer, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return Customer{}, ErrInvalidName
	}

	email = strings.ToLower(strings.TrimSpace(email))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return Customer{}, ErrInvalidEmail
	}

	return Customer{ID: id, Name: name, Email: email, CreatedAt: now}, nil
}
