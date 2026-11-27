package outbox_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/outbox"
)

type placed struct {
	OrderID string `json:"order_id"`
}

func (placed) EventName() string { return "orders.placed.v1" }

type mutableClock struct{ now time.Time }

func (c *mutableClock) Now() time.Time { return c.now }

func TestPublishAndRelayDeliversOnce(t *testing.T) {
	clk := &mutableClock{now: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
	store := outbox.NewMemoryStore()
	bus := eventbus.New()
	pub := outbox.NewPublisher(store, clk)
	relay := outbox.NewRelay(store, bus, clk, slog.New(slog.DiscardHandler), outbox.RelayConfig{})

	var got []string
	bus.Subscribe("orders.placed.v1", "test", eventbus.Typed(func(_ context.Context, _ eventbus.Envelope, evt placed) error {
		got = append(got, evt.OrderID)
		return nil
	}))

	if err := pub.Publish(context.Background(), placed{OrderID: "ord-1"}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatal("event must not be delivered before the relay runs")
	}

	n, err := relay.ProcessBatch(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("ProcessBatch = %d, %v; want 1, nil", n, err)
	}
	if len(got) != 1 || got[0] != "ord-1" {
		t.Fatalf("got %v, want [ord-1]", got)
	}

	if n, err := relay.ProcessBatch(context.Background()); err != nil || n != 0 {
		t.Fatalf("second ProcessBatch = %d, %v; want 0, nil", n, err)
	}
}

func TestRelayRetriesWithBackoff(t *testing.T) {
	clk := &mutableClock{now: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
	store := outbox.NewMemoryStore()
	bus := eventbus.New()
	pub := outbox.NewPublisher(store, clk)
	relay := outbox.NewRelay(store, bus, clk, slog.New(slog.DiscardHandler), outbox.RelayConfig{})

	fail := true
	delivered := 0
	bus.Subscribe("orders.placed.v1", "flaky", func(context.Context, eventbus.Envelope) error {
		if fail {
			return errors.New("temporary error")
		}
		delivered++
		return nil
	})

	if err := pub.Publish(context.Background(), placed{OrderID: "ord-1"}); err != nil {
		t.Fatal(err)
	}

	// First attempt fails and schedules backoff.
	if n, err := relay.ProcessBatch(context.Background()); err != nil || n != 0 {
		t.Fatalf("first batch = %d, %v; want 0, nil", n, err)
	}

	// Immediately retrying does nothing while in backoff.
	fail = false
	if n, err := relay.ProcessBatch(context.Background()); err != nil || n != 0 {
		t.Fatalf("during backoff = %d, %v; want 0, nil", n, err)
	}

	// Advance clock past backoff: delivery succeeds.
	clk.now = clk.now.Add(5 * time.Second)
	if n, err := relay.ProcessBatch(context.Background()); err != nil || n != 1 || delivered != 1 {
		t.Fatalf("after backoff = %d, %v (delivered=%d); want 1, nil", n, err, delivered)
	}
}
