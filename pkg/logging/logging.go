// Package logging configures structured logging and carries request-scoped
// loggers through context.Context.
package logging

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// New builds the process logger. format is "json" or "text".
//
// Every record logged with a context that belongs to a trace carries
// trace_id and span_id, so a log line leads straight to its trace and back.
// That only works with the *Context logging methods; the linter enforces
// their use wherever a context is in scope.
func New(w io.Writer, level slog.Level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler = slog.NewJSONHandler(w, opts)
	if format == "text" {
		handler = slog.NewTextHandler(w, opts)
	}
	return slog.New(traceHandler{handler})
}

// traceHandler adds the active trace and span IDs to every record.
type traceHandler struct {
	slog.Handler
}

func (h traceHandler) Handle(ctx context.Context, record slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, record)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{h.Handler.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{h.Handler.WithGroup(name)}
}

type ctxKey struct{}

// WithContext returns a context carrying logger.
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// FromContext returns the logger stored in ctx, or fallback when there is
// none. Request-scoped loggers already carry attributes such as request_id.
func FromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if logger, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return logger
	}
	return fallback
}
