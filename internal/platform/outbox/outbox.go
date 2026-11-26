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

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/clock"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/event"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/id"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/eventbus"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/postgres"
)

// Schema is the database schema owned by the outbox.
const Schema = "outbox"

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
type Publisher struct {
	db    *postgres.DB
	clock clock.Clock
}

// NewPublisher returns a Publisher writing to db.
func NewPublisher(db *postgres.DB, clk clock.Clock) *Publisher {
	return &Publisher{db: db, clock: clk}
}

// Publish records events as part of the transaction carried by ctx.
func (p *Publisher) Publish(ctx context.Context, events ...event.Event) error {
	if !postgres.InTx(ctx) {
		return ErrNoTransaction
	}
	now := p.clock.Now()
	for _, evt := range events {
		payload, err := json.Marshal(evt)
		if err != nil {
			return fmt.Errorf("outbox: encode %s: %w", evt.EventName(), err)
		}
		_, err = p.db.Conn(ctx).Exec(ctx, `
			INSERT INTO outbox.messages (id, name, payload, occurred_at, available_at)
			VALUES ($1, $2, $3, $4, $4)`,
			id.New(), evt.EventName(), payload, now)
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
	return &Relay{db: db, dispatcher: dispatcher, logger: logger, cfg: cfg}
}

// Run polls until ctx is cancelled. It always returns nil on cancellation so
// it composes with errgroup-style supervisors.
func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		// Drain: keep going while batches come back full.
		for {
			n, err := r.ProcessBatch(ctx)
			if err != nil && ctx.Err() == nil {
				r.logger.ErrorContext(ctx, "outbox relay batch failed", "error", err)
			}
			if err != nil || n < r.cfg.BatchSize {
				break
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

type message struct {
	seq      int64
	attempts int
	env      eventbus.Envelope
}

// ProcessBatch claims up to BatchSize pending messages, dispatches them and
// records the outcome. It returns the number of messages it handled.
func (r *Relay) ProcessBatch(ctx context.Context) (int, error) {
	handled := 0
	err := r.db.WithinTx(ctx, func(txCtx context.Context) error {
		conn := r.db.Conn(txCtx)

		rows, err := conn.Query(txCtx, `
			SELECT seq, id::text, name, payload, occurred_at, attempts
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
			if err := rows.Scan(&m.seq, &m.env.ID, &m.env.Name, &m.env.Payload, &m.env.OccurredAt, &m.attempts); err != nil {
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
			dispatchErr := r.dispatcher.Dispatch(ctx, m.env)
			if err := r.record(txCtx, conn, m, dispatchErr); err != nil {
				return err
			}
			handled++
		}
		return nil
	})
	return handled, err
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
