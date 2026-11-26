// Package app holds the notifications module's use cases.
//
// The module is small, so it has no separate domain package: layers are
// added when the complexity warrants them, not by default.
package app

import (
	"context"
	"fmt"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/id"
)

// Notification is a message sent to a customer.
type Notification struct {
	ID string
	// EventID is the integration event that caused the notification. It is
	// unique, which is what makes event handling idempotent.
	EventID    string
	CustomerID string
	Recipient  string
	Subject    string
	Body       string
	CreatedAt  time.Time
}

// Contact is how to reach a customer.
type Contact struct {
	Name  string
	Email string
}

// Repository is the persistence port for notifications.
type Repository interface {
	// Insert stores n and reports whether it was new. It returns false,
	// without error, when a notification for the same event already exists.
	Insert(ctx context.Context, n Notification) (inserted bool, err error)
}

// CustomerDirectory is what the notifications module needs to know about
// customers.
type CustomerDirectory interface {
	Contact(ctx context.Context, customerID string) (Contact, error)
}

// Sender delivers a notification (email, SMS, push, ...).
type Sender interface {
	Send(ctx context.Context, n Notification) error
}

// Transactor runs a function inside a transaction that is propagated
// through the context.
type Transactor interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Service implements the notifications use cases.
type Service struct {
	repo      Repository
	customers CustomerDirectory
	sender    Sender
	tx        Transactor
	clock     clock.Clock
}

// NewService wires a Service.
func NewService(repo Repository, customers CustomerDirectory, sender Sender, tx Transactor, clk clock.Clock) *Service {
	return &Service{repo: repo, customers: customers, sender: sender, tx: tx, clock: clk}
}

// OrderPlaced is the input of Service.NotifyOrderPlaced, expressed in this
// module's own terms rather than in the publisher's event type.
type OrderPlaced struct {
	EventID     string
	OrderID     string
	CustomerID  string
	TotalAmount int64
	Currency    string
}

// NotifyOrderPlaced sends an order confirmation.
//
// Events are delivered at least once, so this must be idempotent. The
// notification row, unique per event, is the dedupe record: a redelivered
// event finds the row and does nothing. Insert and send share a transaction,
// so a failed send rolls the row back and the retry starts clean.
func (s *Service) NotifyOrderPlaced(ctx context.Context, evt OrderPlaced) error {
	contact, err := s.customers.Contact(ctx, evt.CustomerID)
	if err != nil {
		return fmt.Errorf("notifications: look up customer %s: %w", evt.CustomerID, err)
	}

	n := Notification{
		ID:         id.New(),
		EventID:    evt.EventID,
		CustomerID: evt.CustomerID,
		Recipient:  contact.Email,
		Subject:    "Your order is confirmed",
		Body: fmt.Sprintf("Hi %s, we received your order %s for %d.%02d %s.",
			contact.Name, evt.OrderID, evt.TotalAmount/100, evt.TotalAmount%100, evt.Currency),
		CreatedAt: s.clock.Now(),
	}

	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		inserted, err := s.repo.Insert(ctx, n)
		if err != nil {
			return err
		}
		if !inserted {
			return nil // already handled: duplicate delivery
		}
		return s.sender.Send(ctx, n)
	})
}
