package core_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/clock"
	core "github.com/FeliceGeracitano/go-modular-monolith/modules/notifications/internal"
)

type fakeRepo struct{ byEvent map[string]core.Notification }

func (f *fakeRepo) Exists(_ context.Context, eventID string) (bool, error) {
	_, ok := f.byEvent[eventID]
	return ok, nil
}

func (f *fakeRepo) Save(_ context.Context, n core.Notification) error {
	f.byEvent[n.EventID] = n
	return nil
}

func (f *fakeRepo) List(_ context.Context, customerID string, _, _ int) ([]core.Notification, int, error) {
	var out []core.Notification
	for _, n := range f.byEvent {
		if customerID == "" || n.CustomerID == customerID {
			out = append(out, n)
		}
	}
	return out, len(out), nil
}

type fakeCustomers struct{}

func (fakeCustomers) Contact(context.Context, string) (core.Contact, error) {
	return core.Contact{Name: "Ada", Email: "ada@example.com"}, nil
}

type fakeSender struct {
	sent []core.Notification
	err  error
}

func (f *fakeSender) Send(_ context.Context, n core.Notification) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, n)
	return nil
}

func newService() (*core.Service, *fakeRepo, *fakeSender) {
	repo := &fakeRepo{byEvent: map[string]core.Notification{}}
	sender := &fakeSender{}
	svc := core.NewService(repo, fakeCustomers{}, sender, clock.Fixed(time.Unix(0, 0).UTC()))
	return svc, repo, sender
}

var placed = core.OrderPlaced{EventID: "evt-1", OrderID: "order-1", CustomerID: "cust-1", TotalAmount: 2999, Currency: "EUR"}

func TestNotifyOrderPlacedSendsConfirmation(t *testing.T) {
	svc, _, sender := newService()

	if err := svc.NotifyOrderPlaced(context.Background(), placed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent %d notifications, want 1", len(sender.sent))
	}
	got := sender.sent[0]
	if got.Recipient != "ada@example.com" || !strings.Contains(got.Body, "29.99 EUR") {
		t.Fatalf("unexpected notification: %+v", got)
	}
}

func TestNotifyOrderPlacedIsIdempotent(t *testing.T) {
	svc, _, sender := newService()

	for range 3 {
		if err := svc.NotifyOrderPlaced(context.Background(), placed); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent %d notifications for one event, want 1", len(sender.sent))
	}
}

func TestNotifyOrderPlacedCanBeRetriedAfterSendFailure(t *testing.T) {
	svc, repo, sender := newService()
	sender.err = errors.New("smtp unavailable")

	if err := svc.NotifyOrderPlaced(context.Background(), placed); err == nil {
		t.Fatal("expected send failure to be returned")
	}
	if len(repo.byEvent) != 0 {
		t.Fatal("failed send must not leave a dedupe record behind")
	}

	sender.err = nil
	if err := svc.NotifyOrderPlaced(context.Background(), placed); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent %d notifications after retry, want 1", len(sender.sent))
	}
}

func TestNotifyOrderCancelledSendsCancellation(t *testing.T) {
	svc, _, sender := newService()
	evt := core.OrderCancelled{EventID: "evt-cancel-1", OrderID: "order-1", CustomerID: "cust-1"}

	if err := svc.NotifyOrderCancelled(context.Background(), evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sender.sent) != 1 || !strings.Contains(sender.sent[0].Subject, "cancelled") {
		t.Fatalf("unexpected sent notifications: %+v", sender.sent)
	}
}
