package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/apperr"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/id"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/logging"
)

// Middleware decorates an http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middlewares to h. The first middleware is the outermost.
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

const requestIDHeader = "X-Request-Id"

type requestIDKey struct{}

// RequestIDFrom returns the request ID stored in ctx, if any.
func RequestIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey{}).(string)
	return v
}

// RequestID assigns every request an ID (reusing a well-formed inbound
// X-Request-Id), echoes it in the response and puts a logger carrying it
// into the request context.
func RequestID(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get(requestIDHeader)
			if !id.Valid(requestID) {
				requestID = id.New()
			}
			w.Header().Set(requestIDHeader, requestID)

			ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
			ctx = logging.WithContext(ctx, logger.With("request_id", requestID))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// statusRecorder captures the response status for access logs.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// statusOrOK returns the recorded status. A handler that wrote nothing
// implicitly answered 200.
func (s *statusRecorder) statusOrOK() int {
	if s.status == 0 {
		return http.StatusOK
	}
	return s.status
}

// AccessLog logs one line per request. Probe requests are logged at debug
// level: they arrive every few seconds and say nothing new.
func AccessLog(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}

			next.ServeHTTP(rec, r)

			level := slog.LevelInfo
			if isProbe(r) {
				level = slog.LevelDebug
			}
			// r.Pattern is the matched route ("GET /api/v1/orders/{id}"),
			// which keeps log cardinality bounded.
			logging.FromContext(r.Context(), logger).Log(r.Context(), level, "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"route", r.Pattern,
				"status", rec.statusOrOK(),
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

// Recover turns a handler panic into a 500 response instead of a dropped
// connection, and logs the stack.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			defer func() {
				p := recover()
				if p == nil {
					return
				}
				if err, ok := p.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(p) // the documented way to abort a response
				}
				logging.FromContext(ctx, logger).ErrorContext(ctx, "panic in http handler",
					"panic", p, "stack", string(debug.Stack()))
				Error(w, r, apperr.New(apperr.KindInternal, "internal", "internal error"))
			}()
			next.ServeHTTP(w, r)
		})
	}
}
