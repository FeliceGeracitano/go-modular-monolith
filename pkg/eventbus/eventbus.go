// Package eventbus routes integration events to in-process subscribers.
package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/FeliceGeracitano/go-modular-monolith/pkg/eventbus"

type Envelope struct {
	ID         string
	Name       string
	OccurredAt time.Time
	Payload    json.RawMessage
}

type Handler func(ctx context.Context, env Envelope) error

type Subscriber interface {
	Subscribe(eventName, handlerName string, handler Handler)
}

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

type Bus struct {
	mu   sync.RWMutex
	subs map[string][]subscription
}

func New() *Bus {
	return &Bus{subs: make(map[string][]subscription)}
}

func (b *Bus) Subscribe(eventName, handlerName string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[eventName] = append(b.subs[eventName], subscription{name: handlerName, handler: handler})
}

func (b *Bus) Dispatch(ctx context.Context, env Envelope) error {
	b.mu.RLock()
	subs := append([]subscription(nil), b.subs[env.Name]...)
	b.mu.RUnlock()

	ctx, span := otel.Tracer(instrumentationName).Start(ctx, "deliver "+env.Name,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("event.name", env.Name),
			attribute.String("event.id", env.ID),
		),
	)
	defer span.End()

	var errs []error
	for _, sub := range subs {
		if err := safely(ctx, sub.handler, env); err != nil {
			errs = append(errs, fmt.Errorf("handler %s: %w", sub.name, err))
		}
	}
	err := errors.Join(errs...)
	recordDelivery(ctx, env.Name, err)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return err
}

func recordDelivery(ctx context.Context, eventName string, err error) {
	outcome := "ok"
	if err != nil {
		outcome = "error"
	}
	counter, cErr := otel.Meter(instrumentationName).Int64Counter("eventbus.deliveries")
	if cErr != nil {
		return
	}
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("event.name", eventName),
		attribute.String("outcome", outcome),
	))
}

func safely(ctx context.Context, h Handler, env Envelope) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return h(ctx, env)
}
