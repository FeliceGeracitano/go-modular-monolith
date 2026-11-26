// Package outbox implements the transactional outbox pattern.
//
// Publishing an event and changing business state must be atomic: an order
// that is saved but never announced, or announced but rolled back, leaves
// the system inconsistent. So Publisher does not deliver anything; it stores
// events in the outbox table using the caller's transaction. A Relay then
// reads committed events and dispatches them to subscribers.
//
// Delivery is at-least-once and unordered across retries. Subscribers must
// be idempotent (deduplicate on Envelope.ID).
package outbox

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/event"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/id"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres"
)

// Schema is the database schema owned by the outbox.
const Schema = "outbox"

const instrumentationName = "platform/outbox"

//go:embed migrations/*.sql
var migrations embed.FS

// Migrations returns the outbox's SQL migrations.
func Migrations() fs.FS {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		panic(err) // unreachable: the directory is embedded at build time
	}
	return sub
}

// ErrNoTransaction is returned by Publish when the context carries no
// transaction. Publishing outside the transaction that changes business
// state would silently break the atomicity the outbox exists to provide.
var ErrNoTransaction = errors.New("outbox: publish must be called within a transaction")

// Publisher stores events in the outbox. It satisfies the event publisher
// port that modules declare in their application layer.
//
// It holds no database handle: it writes through the transaction the caller
// already opened, whichever module's connection that is.
type Publisher struct {
	clock clock.Clock
}

// NewPublisher returns a Publisher.
func NewPublisher(clk clock.Clock) *Publisher {
	return &Publisher{clock: clk}
}

// Publish records events as part of the transaction carried by ctx.
func (p *Publisher) Publish(ctx context.Context, events ...event.Event) error {
	tx, ok := postgres.TxFrom(ctx)
	if !ok {
		return ErrNoTransaction
	}

	// Carry the publisher's trace context with the event, so the handlers
	// that eventually run show up in the same trace as the request that
	// caused them.
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	headers, err := json.Marshal(carrier)
	if err != nil {
		return fmt.Errorf("outbox: encode headers: %w", err)
	}

	now := p.clock.Now()
	for _, evt := range events {
		payload, err := json.Marshal(evt)
		if err != nil {
			return fmt.Errorf("outbox: encode %s: %w", evt.EventName(), err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO outbox.messages (id, name, payload, headers, occurred_at, available_at)
			VALUES ($1, $2, $3, $4, $5, $5)`,
			id.New(), evt.EventName(), payload, headers, now)
		if err != nil {
			return fmt.Errorf("outbox: store %s: %w", evt.EventName(), err)
		}
	}
	return nil
}

// Dispatcher delivers an envelope to its subscribers. *eventbus.Bus
// implements it; a broker client could too.
type Dispatcher interface {
	Dispatch(ctx context.Context, env eventbus.Envelope) error
}

// RelayConfig tunes a Relay.
type RelayConfig struct {
	// PollInterval is how often the relay looks for new messages.
	PollInterval time.Duration
	// BatchSize is the maximum number of messages handled per poll.
	BatchSize int
	// MaxAttempts is how many times a message is tried before the relay
	// gives up on it. Abandoned messages stay in the table for inspection.
	MaxAttempts int
}

// Relay moves committed messages from the outbox table to a Dispatcher.
type Relay struct {
	db         *postgres.DB
	dispatcher Dispatcher
	logger     *slog.Logger
	cfg        RelayConfig
	tracer     trace.Tracer
	deliveries metric.Int64Counter
}

// NewRelay returns a Relay. Several relays (one per process instance) may
// run against the same table: rows are claimed with FOR UPDATE SKIP LOCKED.
func NewRelay(db *postgres.DB, dispatcher Dispatcher, logger *slog.Logger, cfg RelayConfig) *Relay {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 10
	}
	// A failed registration yields a no-op counter: telemetry must never
	// stop events from flowing.
	deliveries, _ := otel.Meter(instrumentationName).Int64Counter(
		"outbox.deliveries",
		metric.WithDescription("Outbox delivery attempts, by event and outcome."),
	)
	return &Relay{
		db:         db,
		dispatcher: dispatcher,
		logger:     logger,
		cfg:        cfg,
		tracer:     otel.Tracer(instrumentationName),
		deliveries: deliveries,
	}
}

// Run polls until ctx is cancelled. It always returns nil, so it composes
// with errgroup-style supervisors: a failing database is retried, not fatal.
func (r *Relay) Run(ctx context.Context) error {
	failures := 0
	for {
		wait := r.poll(ctx, &failures)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
	}
}

// poll drains the outbox once and returns how long to wait before the next
// poll. failures counts consecutive failed polls.
func (r *Relay) poll(ctx context.Context, failures *int) time.Duration {
	// Drain: keep going while batches come back full.
	var err error
	for {
		var n int
		n, err = r.ProcessBatch(ctx)
		if err != nil || n < r.cfg.BatchSize {
			break
		}
	}
	if err == nil || ctx.Err() != nil {
		*failures = 0
		return r.cfg.PollInterval
	}

	// The database is unreachable or not migrated. Keep trying, but back
	// off so an outage is not also a flood of logs and load.
	*failures++
	wait := min(r.cfg.PollInterval<<min(*failures, 10), maxPollBackoff)
	r.logger.ErrorContext(ctx, "outbox relay batch failed",
		"error", err, "consecutive_failures", *failures, "retry_in", wait.String())
	return wait
}

// maxPollBackoff bounds how long the relay waits between attempts while the
// database is failing.
const maxPollBackoff = 30 * time.Second

type message struct {
	seq      int64
	attempts int
	headers  []byte
	env      eventbus.Envelope
}

// ProcessBatch claims up to BatchSize pending messages, dispatches them and
// records the outcome. It returns the number of messages it handled.
func (r *Relay) ProcessBatch(ctx context.Context) (int, error) {
	handled := 0
	err := r.db.WithinTx(ctx, func(txCtx context.Context) error {
		conn := r.db.Conn(txCtx)

		rows, err := conn.Query(txCtx, `
			SELECT seq, id::text, name, payload, headers, occurred_at, attempts
			FROM outbox.messages
			WHERE processed_at IS NULL AND attempts < $1 AND available_at <= now()
			ORDER BY seq
			LIMIT $2
			FOR UPDATE SKIP LOCKED`, r.cfg.MaxAttempts, r.cfg.BatchSize)
		if err != nil {
			return fmt.Errorf("outbox: claim: %w", err)
		}
		var batch []message
		for rows.Next() {
			var m message
			if err := rows.Scan(&m.seq, &m.env.ID, &m.env.Name, &m.env.Payload, &m.headers, &m.env.OccurredAt, &m.attempts); err != nil {
				rows.Close()
				return fmt.Errorf("outbox: scan: %w", err)
			}
			batch = append(batch, m)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("outbox: read: %w", err)
		}

		for _, m := range batch {
			// Handlers get the caller's context, not txCtx: they manage
			// their own transactions and must not join the relay's.
			dispatchErr := r.deliver(ctx, m)
			if err := r.record(txCtx, conn, m, dispatchErr); err != nil {
				return err
			}
			handled++
		}
		return nil
	})
	return handled, err
}

// deliver dispatches one message inside a span that continues the
// publisher's trace.
func (r *Relay) deliver(ctx context.Context, m message) error {
	carrier := propagation.MapCarrier{}
	_ = json.Unmarshal(m.headers, &carrier) // headers are best-effort metadata

	ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
	ctx, span := r.tracer.Start(ctx, "deliver "+m.env.Name,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("messaging.system", "outbox"),
			attribute.String("messaging.operation.type", "process"),
			attribute.String("messaging.destination.name", m.env.Name),
			attribute.String("messaging.message.id", m.env.ID),
		))
	defer span.End()

	err := r.dispatcher.Dispatch(ctx, m.env)

	outcome := "delivered"
	if err != nil {
		outcome = "failed"
		span.RecordError(err)
		span.SetStatus(codes.Error, "delivery failed")
	}
	r.deliveries.Add(ctx, 1, metric.WithAttributes(
		attribute.String("event", m.env.Name),
		attribute.String("outcome", outcome),
	))
	return err
}

func (r *Relay) record(ctx context.Context, conn postgres.Querier, m message, dispatchErr error) error {
	if dispatchErr == nil {
		if _, err := conn.Exec(ctx,
			`UPDATE outbox.messages SET processed_at = now(), attempts = attempts + 1, last_error = NULL WHERE seq = $1`,
			m.seq); err != nil {
			return fmt.Errorf("outbox: mark processed: %w", err)
		}
		return nil
	}

	attempts := m.attempts + 1
	level := slog.LevelWarn
	if attempts >= r.cfg.MaxAttempts {
		level = slog.LevelError // abandoned: needs a human
	}
	r.logger.Log(ctx, level, "outbox delivery failed",
		"event_id", m.env.ID, "event_name", m.env.Name,
		"attempt", attempts, "max_attempts", r.cfg.MaxAttempts, "error", dispatchErr)

	if _, err := conn.Exec(ctx, `
		UPDATE outbox.messages
		SET attempts = $2, last_error = $3, available_at = now() + $4::interval
		WHERE seq = $1`,
		m.seq, attempts, dispatchErr.Error(), backoff(attempts).String()); err != nil {
		return fmt.Errorf("outbox: mark failed: %w", err)
	}
	return nil
}

// backoff grows quadratically with the attempt number, capped at 5 minutes.
func backoff(attempt int) time.Duration {
	return min(time.Duration(attempt*attempt)*time.Second, 5*time.Minute)
}
