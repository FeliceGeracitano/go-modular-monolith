package eventbus_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/eventbus"
)

type greeted struct {
	Name string `json:"name"`
}

func TestDispatchDeliversToEverySubscriber(t *testing.T) {
	bus := eventbus.New()
	var got []string

	bus.Subscribe("demo.greeted.v1", "first", eventbus.Typed(func(_ context.Context, _ eventbus.Envelope, evt greeted) error {
		got = append(got, "first:"+evt.Name)
		return nil
	}))
	bus.Subscribe("demo.greeted.v1", "second", eventbus.Typed(func(_ context.Context, _ eventbus.Envelope, evt greeted) error {
		got = append(got, "second:"+evt.Name)
		return nil
	}))

	err := bus.Dispatch(context.Background(), eventbus.Envelope{Name: "demo.greeted.v1", Payload: []byte(`{"name":"Ada"}`)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(got, ",") != "first:Ada,second:Ada" {
		t.Fatalf("got %v", got)
	}
}

func TestDispatchRunsAllHandlersAndJoinsFailures(t *testing.T) {
	bus := eventbus.New()
	boom := errors.New("boom")
	ran := false

	bus.Subscribe("demo.greeted.v1", "failing", func(context.Context, eventbus.Envelope) error { return boom })
	bus.Subscribe("demo.greeted.v1", "panicking", func(context.Context, eventbus.Envelope) error { panic("oops") })
	bus.Subscribe("demo.greeted.v1", "healthy", func(context.Context, eventbus.Envelope) error { ran = true; return nil })

	err := bus.Dispatch(context.Background(), eventbus.Envelope{Name: "demo.greeted.v1"})

	if !errors.Is(err, boom) {
		t.Fatalf("error should wrap the handler failure, got: %v", err)
	}
	if !strings.Contains(err.Error(), "handler panicking: panic: oops") {
		t.Fatalf("panic should be reported as an error, got: %v", err)
	}
	if !ran {
		t.Fatal("a failing handler must not prevent the others from running")
	}
}

func TestDispatchWithoutSubscribersIsNotAnError(t *testing.T) {
	if err := eventbus.New().Dispatch(context.Background(), eventbus.Envelope{Name: "nobody.cares.v1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
