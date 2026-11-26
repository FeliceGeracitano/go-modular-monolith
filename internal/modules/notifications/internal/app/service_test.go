package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/modules/notifications/internal/app"
)

type fakeRepo struct{ byEvent map[string]app.Notification }

func (f *fakeRepo) Insert(_ context.Context, n app.Notification) (bool, error) {
	if _, exists := f.byEvent[n.EventID]; exists {
		return false, nil
	}
	f.byEvent[n.EventID] = n
	return true, nil
}

type fakeCustomers struct{}

func (fakeCustomers) Contact(context.Context, string) (app.Contact, error) {
	return app.Contact{Name: "Ada", Email: "ada@example.com"}, nil
}

type fakeSender struct {
	sent []app.Notification
	err  error
}

func (f *fakeSender) Send(_ context.Context, n app.Notification) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, n)
	return nil
}

// rollbackTx mimics a real transaction: when fn fails, writes are undone.
type rollbackTx struct{ repo *fakeRepo }

func (t rollbackTx) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	snapshot := make(map[string]app.Notification, len(t.repo.byEvent))
	for k, v := range t.repo.byEvent {
		snapshot[k] = v
	}
	if err := fn(ctx); err != nil {
		t.repo.byEvent = snapshot
		return err
	}
	return nil
}

func newService() (*app.Service, *fakeRepo, *fakeSender) {
	repo := &fakeRepo{byEvent: map[string]app.Notification{}}
	sender := &fakeSender{}
	svc := app.NewService(repo, fakeCustomers{}, sender, rollbackTx{repo}, clock.Fixed(time.Unix(0, 0).UTC()))
	return svc, repo, sender
}

var placed = app.OrderPlaced{EventID: "evt-1", OrderID: "order-1", CustomerID: "cust-1", TotalAmount: 2999, Currency: "EUR"}

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

	for range 3 { // at-least-once delivery: the same event arrives again
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
		t.Fatal("expected the send failure to be returned so the event is redelivered")
	}
	if len(repo.byEvent) != 0 {
		t.Fatal("a failed send must not leave a dedupe record behind")
	}

	sender.err = nil
	if err := svc.NotifyOrderPlaced(context.Background(), placed); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent %d notifications after retry, want 1", len(sender.sent))
	}
}
