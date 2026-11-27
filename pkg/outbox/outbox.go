// Package outbox implements the outbox pattern behind a storage-agnostic Store
// interface, with an in-memory default and an asynchronous Relay.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/event"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/id"
	"github.com/FeliceGeracitano/go-modular-monolith/pkg/eventbus"
)

type Message struct {
	ID            string
	Name          string
	Payload       json.RawMessage
	TraceHeaders  map[string]string
	OccurredAt    time.Time
	Attempts      int
	NextAttemptAt time.Time
	ProcessedAt   *time.Time
	LastError     string
}

// Store persists outbox messages. Swap MemoryStore for a database-backed
// implementation to make publishing atomic with state writes.
type Store interface {
	Append(ctx context.Context, msgs ...Message) error
	ClaimPending(ctx context.Context, now time.Time, limit int) ([]Message, error)
	MarkProcessed(ctx context.Context, id string, at time.Time) error
	MarkFailed(ctx context.Context, id string, errMsg string, nextAttempt time.Time) error
}

type Dispatcher interface {
	Dispatch(ctx context.Context, env eventbus.Envelope) error
}

type MemoryStore struct {
	mu   sync.Mutex
	msgs []Message
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) Append(_ context.Context, msgs ...Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.msgs = append(s.msgs, msgs...)
	return nil
}

func (s *MemoryStore) ClaimPending(_ context.Context, now time.Time, limit int) ([]Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var batch []Message
	for _, m := range s.msgs {
		if m.ProcessedAt != nil || m.NextAttemptAt.After(now) {
			continue
		}
		batch = append(batch, m)
		if len(batch) >= limit {
			break
		}
	}
	return batch, nil
}

func (s *MemoryStore) MarkProcessed(_ context.Context, msgID string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.msgs {
		if s.msgs[i].ID == msgID {
			t := at
			s.msgs[i].ProcessedAt = &t
			s.msgs[i].LastError = ""
			return nil
		}
	}
	return nil
}

func (s *MemoryStore) MarkFailed(_ context.Context, msgID string, errMsg string, nextAttempt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.msgs {
		if s.msgs[i].ID == msgID {
			s.msgs[i].Attempts++
			s.msgs[i].LastError = errMsg
			s.msgs[i].NextAttemptAt = nextAttempt
			return nil
		}
	}
	return nil
}

type Publisher struct {
	store Store
	clock clock.Clock
}

func NewPublisher(store Store, clk clock.Clock) *Publisher {
	return &Publisher{store: store, clock: clk}
}

func (p *Publisher) Publish(ctx context.Context, events ...event.Event) error {
	if len(events) == 0 {
		return nil
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	now := p.clock.Now()
	msgs := make([]Message, 0, len(events))
	for _, evt := range events {
		payload, err := json.Marshal(evt)
		if err != nil {
			return fmt.Errorf("outbox: marshal %s: %w", evt.EventName(), err)
		}
		msgs = append(msgs, Message{
			ID:            id.New(),
			Name:          evt.EventName(),
			Payload:       payload,
			TraceHeaders:  map[string]string(carrier),
			OccurredAt:    now,
			NextAttemptAt: now,
		})
	}
	return p.store.Append(ctx, msgs...)
}

type RelayConfig struct {
	PollInterval time.Duration
	BatchSize    int
	MaxAttempts  int
}

type Relay struct {
	store      Store
	dispatcher Dispatcher
	clock      clock.Clock
	logger     *slog.Logger
	cfg        RelayConfig
}

func NewRelay(store Store, dispatcher Dispatcher, clk clock.Clock, logger *slog.Logger, cfg RelayConfig) *Relay {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 100 * time.Millisecond
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 5
	}
	return &Relay{store: store, dispatcher: dispatcher, clock: clk, logger: logger, cfg: cfg}
}

func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		if _, err := r.ProcessBatch(ctx); err != nil && ctx.Err() == nil {
			r.logger.ErrorContext(ctx, "outbox relay error", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *Relay) ProcessBatch(ctx context.Context) (int, error) {
	now := r.clock.Now()
	batch, err := r.store.ClaimPending(ctx, now, r.cfg.BatchSize)
	if err != nil {
		return 0, err
	}

	processed := 0
	for _, m := range batch {
		if m.Attempts >= r.cfg.MaxAttempts {
			continue
		}
		deliverCtx := otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(m.TraceHeaders))
		env := eventbus.Envelope{
			ID:         m.ID,
			Name:       m.Name,
			OccurredAt: m.OccurredAt,
			Payload:    m.Payload,
		}
		if err := r.dispatcher.Dispatch(deliverCtx, env); err != nil {
			backoff := time.Duration(1<<min(m.Attempts, 6)) * time.Second
			if markErr := r.store.MarkFailed(ctx, m.ID, err.Error(), now.Add(backoff)); markErr != nil {
				return processed, markErr
			}
			r.logger.WarnContext(deliverCtx, "outbox delivery failed",
				"event_id", m.ID, "event", m.Name, "attempt", m.Attempts+1, "error", err)
			continue
		}
		if err := r.store.MarkProcessed(ctx, m.ID, r.clock.Now()); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}
