// Package apperr is the error vocabulary shared by every module.
//
// Domain and application code return *Error values to describe *what kind* of
// failure happened (invalid input, not found, conflict, ...). Delivery
// adapters (HTTP, gRPC, CLI) translate the Kind into a transport-specific
// status. Anything that is not an *Error is treated as an internal failure
// and its details are never exposed to clients.
package apperr

import (
	"errors"
	"fmt"
)

// Kind classifies an error independently of any transport.
type Kind uint8

// The supported error kinds. KindInternal is the zero value on purpose: an
// error nobody classified is an internal error.
const (
	KindInternal Kind = iota
	KindInvalid
	KindNotFound
	KindConflict
	KindUnauthorized
	KindForbidden
)

// String returns a human readable name for the kind.
func (k Kind) String() string {
	switch k {
	case KindInvalid:
		return "invalid"
	case KindNotFound:
		return "not_found"
	case KindConflict:
		return "conflict"
	case KindUnauthorized:
		return "unauthorized"
	case KindForbidden:
		return "forbidden"
	default:
		return "internal"
	}
}

// Error is a classified application error.
type Error struct {
	// Kind drives how adapters report the error.
	Kind Kind
	// Code is a stable, machine readable identifier, namespaced by module,
	// e.g. "customers.email_taken". Clients may branch on it.
	Code string
	// Message is safe to show to clients.
	Message string

	cause error
}

// New builds a classified error.
func New(kind Kind, code, message string) *Error {
	return &Error{Kind: kind, Code: code, Message: message}
}

// Invalid reports input that violates a business or validation rule.
func Invalid(code, message string) *Error { return New(KindInvalid, code, message) }

// NotFound reports a missing resource.
func NotFound(code, message string) *Error { return New(KindNotFound, code, message) }

// Conflict reports a state conflict, such as a uniqueness violation.
func Conflict(code, message string) *Error { return New(KindConflict, code, message) }

// Error implements error.
func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.cause)
	}
	return e.Code + ": " + e.Message
}

// Unwrap exposes the underlying cause to errors.Is / errors.As.
func (e *Error) Unwrap() error { return e.cause }

// Is makes errors.Is(err, target) match on Code, so sentinel errors keep
// matching after WithCause produced a copy.
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return e.Kind == t.Kind && e.Code == t.Code
}

// WithCause returns a copy of e that wraps cause. The receiver is not
// modified, so package-level sentinel errors stay immutable.
func (e *Error) WithCause(cause error) *Error {
	clone := *e
	clone.cause = cause
	return &clone
}

// As extracts the first *Error in err's chain.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// KindOf returns the Kind of err, or KindInternal when err is unclassified.
func KindOf(err error) Kind {
	if e, ok := As(err); ok {
		return e.Kind
	}
	return KindInternal
}
