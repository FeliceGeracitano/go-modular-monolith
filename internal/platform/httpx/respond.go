// Package httpx holds the HTTP plumbing shared by every module: JSON
// encoding and decoding, error responses, middleware, health checks and the
// server lifecycle.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/apperr"
	"github.com/FeliceGeracitano/go-modular-monolith/internal/platform/logging"
)

// maxBodyBytes bounds request bodies accepted by Decode.
const maxBodyBytes = 1 << 20 // 1 MiB

// ErrMalformedBody is returned by Decode for bodies that are not the
// expected JSON document.
var ErrMalformedBody = apperr.Invalid("http.malformed_body", "request body must be a single valid JSON object with known fields")

// JSON writes v as a JSON response.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // the client went away; nothing to do
}

// Decode reads a JSON request body into dst. It rejects unknown fields,
// trailing data and bodies larger than 1 MiB.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return ErrMalformedBody.WithCause(err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return ErrMalformedBody
	}
	return nil
}

// Problem is the error response body, following RFC 9457 (problem details).
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Detail    string `json:"detail,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// Error writes err as a problem+json response.
//
//   - Classified errors (*apperr.Error) map to 4xx responses carrying their
//     code and message.
//   - A request that ran out of time maps to 504, and one the client
//     abandoned to 408 (which the client will usually never read).
//   - Anything else is logged and reported as an opaque 500, so internal
//     details never leak to clients.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	logger := logging.FromContext(ctx, slog.Default())
	problem := Problem{Type: "about:blank", RequestID: RequestIDFrom(ctx)}

	appErr, classified := apperr.As(err)
	switch {
	case classified && appErr.Kind != apperr.KindInternal:
		problem.Status = statusFor(appErr.Kind)
		problem.Code = appErr.Code
		problem.Detail = appErr.Message
	case errors.Is(err, context.DeadlineExceeded):
		problem.Status = http.StatusGatewayTimeout
		problem.Code = "timeout"
		logger.WarnContext(ctx, "request timed out", "error", err)
	case errors.Is(err, context.Canceled):
		problem.Status = http.StatusRequestTimeout
		problem.Code = "canceled"
		logger.InfoContext(ctx, "request canceled", "error", err)
	default:
		problem.Status = http.StatusInternalServerError
		problem.Code = "internal"
		logger.ErrorContext(ctx, "request failed", "error", err)
	}
	problem.Title = http.StatusText(problem.Status)

	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	w.WriteHeader(problem.Status)
	_ = json.NewEncoder(w).Encode(problem)
}

func statusFor(kind apperr.Kind) int {
	switch kind {
	case apperr.KindInvalid:
		return http.StatusUnprocessableEntity
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindConflict:
		return http.StatusConflict
	case apperr.KindUnauthorized:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}
