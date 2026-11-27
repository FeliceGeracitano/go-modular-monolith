// Package telemetry sets up OpenTelemetry tracing and metrics.
//
// Instrumentation lives at the edges of the system, in the platform:
// inbound HTTP (httpx), SQL (postgres) and event delivery (outbox). Business
// code is not instrumented and never imports OpenTelemetry; it only has to
// pass its context.Context along, which it does anyway.
//
// Export uses OTLP over HTTP and is configured with the standard OTEL_*
// environment variables (OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_TRACES_SAMPLER,
// ...), so any OpenTelemetry-compatible backend works without code changes.
package telemetry

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Config identifies this process to the telemetry backend.
type Config struct {
	// Enabled turns export on. When false, instrumentation still runs
	// against no-op providers, which costs next to nothing.
	Enabled bool
	Service string
	Version string
	Env     string
}

// Setup installs the global tracer and meter providers and returns a
// function that flushes and stops them. Call it once, before anything that
// creates spans, and call the returned function during shutdown.
func Setup(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error) {
	// Always propagate W3C trace context, so this service joins traces
	// started upstream even while its own export is disabled.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	// Attributes from OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES win over
	// the defaults derived from the build.
	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("service.name", cfg.Service),
			attribute.String("service.version", cfg.Version),
			attribute.String("deployment.environment.name", cfg.Env),
		),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: resource: %w", err)
	}

	traceExporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("telemetry: trace exporter: %w", err)
	}
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)

	metricExporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		_ = tracerProvider.Shutdown(ctx)
		return nil, fmt.Errorf("telemetry: metric exporter: %w", err)
	}
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)

	return func(ctx context.Context) error {
		return errors.Join(tracerProvider.Shutdown(ctx), meterProvider.Shutdown(ctx))
	}, nil
}
