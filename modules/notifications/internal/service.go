package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/id"
)

type Notification struct {
	ID         string
	EventID    string // deduplication key for at-least-once delivery
	CustomerID string
	Recipient  string
	Subject    string
	Body       string
	CreatedAt  time.Time
}

type Contact struct {
	Name  string
	Email string
}

type Repository interface {
	Exists(ctx context.Context, eventID string) (bool, error)
	Save(ctx context.Context, n Notification) error
	List(ctx context.Context, customerID string, limit, offset int) ([]Notification, int, error)
}

type CustomerDirectory interface {
	Contact(ctx context.Context, customerID string) (Contact, error)
}

type Sender interface {
	Send(ctx context.Context, n Notification) error
}

type Service struct {
	repo      Repository
	customers CustomerDirectory
	sender    Sender
	clock     clock.Clock
}

func NewService(repo Repository, customers CustomerDirectory, sender Sender, clk clock.Clock) *Service {
	return &Service{repo: repo, customers: customers, sender: sender, clock: clk}
}

type OrderPlaced struct {
	EventID     string
	OrderID     string
	CustomerID  string
	TotalAmount int64
	Currency    string
}

type OrderCancelled struct {
	EventID    string
	OrderID    string
	CustomerID string
}

type ListResult struct {
	Items  []Notification
	Total  int
	Limit  int
	Offset int
}

func (s *Service) NotifyOrderPlaced(ctx context.Context, evt OrderPlaced) error {
	return s.sendOnce(ctx, evt.EventID, evt.CustomerID, "Your order is confirmed", func(c Contact) string {
		return fmt.Sprintf("Hi %s, we received your order %s for %d.%02d %s.",
			c.Name, evt.OrderID, evt.TotalAmount/100, evt.TotalAmount%100, evt.Currency)
	})
}

func (s *Service) NotifyOrderCancelled(ctx context.Context, evt OrderCancelled) error {
	return s.sendOnce(ctx, evt.EventID, evt.CustomerID, "Your order was cancelled", func(c Contact) string {
		return fmt.Sprintf("Hi %s, your order %s has been cancelled.", c.Name, evt.OrderID)
	})
}

func (s *Service) List(ctx context.Context, customerID string, limit, offset int) (ListResult, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	items, total, err := s.repo.List(ctx, customerID, limit, offset)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) sendOnce(ctx context.Context, eventID, customerID, subject string, body func(Contact) string) error {
	alreadySent, err := s.repo.Exists(ctx, eventID)
	if err != nil {
		return err
	}
	if alreadySent {
		return nil
	}

	contact, err := s.customers.Contact(ctx, customerID)
	if err != nil {
		return fmt.Errorf("notifications: look up customer %s: %w", customerID, err)
	}

	n := Notification{
		ID:         id.New(),
		EventID:    eventID,
		CustomerID: customerID,
		Recipient:  contact.Email,
		Subject:    subject,
		Body:       body(contact),
		CreatedAt:  s.clock.Now(),
	}

	if err := s.sender.Send(ctx, n); err != nil {
		return err
	}
	return s.repo.Save(ctx, n)
}

func sortNotifications(items []Notification) {
	slices.SortFunc(items, func(a, b Notification) int {
		if cmp := a.CreatedAt.Compare(b.CreatedAt); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ID, b.ID)
	})
}
