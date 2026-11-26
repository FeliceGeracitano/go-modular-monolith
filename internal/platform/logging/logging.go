// Package logging configures structured logging and carries request-scoped
// loggers through context.Context.
package logging

import (
	"context"
	"io"
	"log/slog"
)

// New builds the process logger. format is "json" (default) or "text".
func New(w io.Writer, level slog.Level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	if format == "text" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
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
