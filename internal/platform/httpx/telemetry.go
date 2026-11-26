package httpx

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "platform/httpx"

// Telemetry traces every request and records its duration.
//
// It continues the caller's trace when the request carries a traceparent
// header, and names spans and metrics after the matched route pattern
// ("GET /api/v1/orders/{id}"), never the raw path, to keep cardinality
// bounded. Probe endpoints are not instrumented.
func Telemetry() Middleware {
	tracer := otel.Tracer(instrumentationName)
	// A failed instrument registration yields a no-op instrument, which is
	// the right degradation for telemetry: never break request handling.
	duration, _ := otel.Meter(instrumentationName).Float64Histogram(
		"http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of inbound HTTP requests."),
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isProbe(r) {
				next.ServeHTTP(w, r)
				return
			}

			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := tracer.Start(ctx, r.Method, trace.WithSpanKind(trace.SpanKindServer))
			defer span.End()

			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			r = r.WithContext(ctx)
			next.ServeHTTP(rec, r)

			// The router fills in r.Pattern while serving, so the route is
			// only known now. Unmatched requests have none.
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			status := rec.statusOrOK()
			attrs := []attribute.KeyValue{
				attribute.String("http.request.method", r.Method),
				attribute.String("http.route", route),
				attribute.Int("http.response.status_code", status),
			}

			span.SetName(route)
			span.SetAttributes(attrs...)
			if status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(status))
			}
			duration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(attrs...))
		})
	}
}

func isProbe(r *http.Request) bool {
	return r.URL.Path == "/healthz" || r.URL.Path == "/readyz"
}
