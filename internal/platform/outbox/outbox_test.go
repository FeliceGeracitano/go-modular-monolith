package outbox_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/outbox"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres/pgtest"
)

type greeted struct {
	Name string `json:"name"`
}

func (greeted) EventName() string { return "demo.greeted.v1" }

type harness struct {
	db        *postgres.DB
	bus       *eventbus.Bus
	publisher *outbox.Publisher
	relay     *outbox.Relay
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db := pgtest.Open(t)
	if err := postgres.Migrate(context.Background(), db, outbox.Schema, outbox.Migrations()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// The relay runs with the outbox's own restricted role, as in production.
	relayDB, err := db.Scoped(outbox.Schema)
	if err != nil {
		t.Fatalf("scoped: %v", err)
	}
	t.Cleanup(relayDB.Close)

	bus := eventbus.New()
	logger := slog.New(slog.DiscardHandler)
	return &harness{
		db:        db,
		bus:       bus,
		publisher: outbox.NewPublisher(clock.System{}),
		relay:     outbox.NewRelay(relayDB, bus, logger, outbox.RelayConfig{MaxAttempts: 2}),
	}
}

func (h *harness) publish(t *testing.T, name string) {
	t.Helper()
	err := h.db.WithinTx(context.Background(), func(ctx context.Context) error {
		return h.publisher.Publish(ctx, greeted{Name: name})
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func (h *harness) process(t *testing.T) int {
	t.Helper()
	n, err := h.relay.ProcessBatch(context.Background())
	if err != nil {
		t.Fatalf("process batch: %v", err)
	}
	return n
}

func TestPublishRequiresTransaction(t *testing.T) {
	h := newHarness(t)

	err := h.publisher.Publish(context.Background(), greeted{Name: "Ada"})

	if !errors.Is(err, outbox.ErrNoTransaction) {
		t.Fatalf("error = %v, want ErrNoTransaction", err)
	}
}

func TestRelayDeliversCommittedEventsOnce(t *testing.T) {
	h := newHarness(t)
	var got []string
	h.bus.Subscribe("demo.greeted.v1", "test", eventbus.Typed(func(_ context.Context, env eventbus.Envelope, evt greeted) error {
		if env.ID == "" || env.OccurredAt.IsZero() {
			t.Errorf("envelope metadata missing: %+v", env)
		}
		got = append(got, evt.Name)
		return nil
	}))

	h.publish(t, "Ada")
	h.publish(t, "Grace")

	if n := h.process(t); n != 2 {
		t.Fatalf("first batch handled %d messages, want 2", n)
	}
	if n := h.process(t); n != 0 {
		t.Fatalf("second batch handled %d messages, want 0", n)
	}
	if len(got) != 2 || got[0] != "Ada" || got[1] != "Grace" {
		t.Fatalf("delivered %v, want [Ada Grace] in publication order", got)
	}
}

func TestRolledBackEventsAreNeverDelivered(t *testing.T) {
	h := newHarness(t)
	delivered := 0
	h.bus.Subscribe("demo.greeted.v1", "test", func(context.Context, eventbus.Envelope) error {
		delivered++
		return nil
	})

	boom := errors.New("business rule failed after publishing")
	err := h.db.WithinTx(context.Background(), func(ctx context.Context) error {
		if err := h.publisher.Publish(ctx, greeted{Name: "Ada"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the business error", err)
	}

	if n := h.process(t); n != 0 || delivered != 0 {
		t.Fatalf("handled %d, delivered %d; a rolled back event must vanish", n, delivered)
	}
}

func TestFailedDeliveryIsRetriedThenAbandoned(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	calls := 0
	h.bus.Subscribe("demo.greeted.v1", "flaky", func(context.Context, eventbus.Envelope) error {
		calls++
		return errors.New("downstream unavailable")
	})
	makeDue := func() {
		t.Helper()
		if _, err := h.db.Pool().Exec(ctx, `UPDATE outbox.messages SET available_at = now()`); err != nil {
			t.Fatal(err)
		}
	}

	h.publish(t, "Ada")

	// First attempt fails: the message stays pending, with a backoff.
	h.process(t)
	if n := h.process(t); n != 0 {
		t.Fatalf("message was retried before its backoff elapsed (handled %d)", n)
	}

	// Second attempt reaches MaxAttempts: the message is abandoned.
	makeDue()
	h.process(t)
	makeDue()
	if n := h.process(t); n != 0 {
		t.Fatalf("message was retried beyond MaxAttempts (handled %d)", n)
	}
	if calls != 2 {
		t.Fatalf("handler called %d times, want 2", calls)
	}

	var (
		attempts  int
		lastError string
		pending   bool
	)
	err := h.db.Pool().QueryRow(ctx,
		`SELECT attempts, last_error, processed_at IS NULL FROM outbox.messages`).Scan(&attempts, &lastError, &pending)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || !pending || lastError == "" {
		t.Fatalf("attempts=%d pending=%v last_error=%q; want 2, true and the failure recorded", attempts, pending, lastError)
	}
}
