package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "platform/postgres"

// queryTracer records one client span per SQL statement.
//
// Statements are only traced when they belong to an existing trace (an HTTP
// request, an event delivery). Background work such as the outbox poller
// would otherwise emit a root span every second.
type queryTracer struct{}

var _ pgx.QueryTracer = queryTracer{}

func (queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if !trace.SpanContextFromContext(ctx).IsValid() {
		return ctx
	}
	// Queries are parameterised ($1, $2, ...), so the statement text holds
	// no user data and is safe to record.
	ctx, _ = otel.Tracer(tracerName).Start(ctx, operation(data.SQL),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system.name", "postgresql"),
			attribute.String("db.query.text", data.SQL),
		))
	return context.WithValue(ctx, tracedKey{}, true)
}

func (queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if traced, _ := ctx.Value(tracedKey{}).(bool); !traced {
		return
	}
	span := trace.SpanFromContext(ctx)
	if data.Err != nil {
		span.RecordError(data.Err)
		span.SetStatus(codes.Error, "query failed")
	}
	span.End()
}

type tracedKey struct{}

// operation names a span after the statement's verb ("SELECT", "INSERT"),
// which keeps span names low-cardinality.
func operation(sql string) string {
	verb, _, _ := strings.Cut(strings.TrimSpace(sql), " ")
	verb = strings.ToUpper(strings.TrimSpace(verb))
	if verb == "" {
		return "QUERY"
	}
	return verb
}
