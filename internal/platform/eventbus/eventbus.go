// Package eventbus routes integration events to in-process subscribers.
//
// It is deliberately tiny: an Envelope (the wire form of an event) and a Bus
// that fans an envelope out to the handlers registered for its name. Events
// reach the bus through the transactional outbox (see package outbox), never
// directly from business code, so a published event is never lost and never
// announced for a transaction that rolled back.
//
// Replacing the in-process Bus with a real broker (NATS, Kafka, SQS, ...)
// only touches this package and the outbox relay: modules depend on the
// Subscriber interface and on their own publisher port.
package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Envelope is an event in transit: routing metadata plus a JSON payload.
type Envelope struct {
	// ID uniquely identifies the event. Delivery is at-least-once, so
	// handlers use it to deduplicate.
	ID string
	// Name is the versioned event name, e.g. "orders.order_placed.v1".
	Name string
	// OccurredAt is when the publishing transaction recorded the event.
	OccurredAt time.Time
	// Payload is the JSON encoding of the event.
	Payload json.RawMessage
}

// Handler consumes an event. Returning an error asks for redelivery, so
// handlers must be idempotent.
type Handler func(ctx context.Context, env Envelope) error

// Subscriber is what modules use to register their event handlers.
type Subscriber interface {
	// Subscribe registers handler for eventName. handlerName identifies the
	// subscription in logs and errors, e.g. "notifications.order_placed".
	Subscribe(eventName, handlerName string, handler Handler)
}

// Typed adapts a strongly typed handler to a Handler by decoding the payload
// into T.
func Typed[T any](fn func(ctx context.Context, env Envelope, evt T) error) Handler {
	return func(ctx context.Context, env Envelope) error {
		var evt T
		if err := json.Unmarshal(env.Payload, &evt); err != nil {
			return fmt.Errorf("decode %s: %w", env.Name, err)
		}
		return fn(ctx, env, evt)
	}
}

type subscription struct {
	name    string
	handler Handler
}

// Bus is an in-process, synchronous event router. It is safe for concurrent
// use.
type Bus struct {
	mu   sync.RWMutex
	subs map[string][]subscription
}

// New returns an empty Bus.
func New() *Bus {
	return &Bus{subs: make(map[string][]subscription)}
}

// Subscribe implements Subscriber.
func (b *Bus) Subscribe(eventName, handlerName string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[eventName] = append(b.subs[eventName], subscription{name: handlerName, handler: handler})
}

// Dispatch delivers env to every handler subscribed to its name. All
// handlers run even if one fails; their errors are joined. An event nobody
// subscribed to is not an error.
func (b *Bus) Dispatch(ctx context.Context, env Envelope) error {
	b.mu.RLock()
	subs := b.subs[env.Name]
	b.mu.RUnlock()

	var errs []error
	for _, sub := range subs {
		if err := safely(ctx, sub.handler, env); err != nil {
			errs = append(errs, fmt.Errorf("handler %s: %w", sub.name, err))
		}
	}
	return errors.Join(errs...)
}

// safely turns a handler panic into an error so one faulty subscriber cannot
// take the dispatcher down.
func safely(ctx context.Context, h Handler, env Envelope) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return h(ctx, env)
}
