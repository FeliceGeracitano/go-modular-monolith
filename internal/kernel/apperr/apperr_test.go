package apperr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/apperr"
)

func TestKindOf(t *testing.T) {
	sentinel := apperr.NotFound("things.not_found", "thing not found")

	tests := []struct {
		name string
		err  error
		want apperr.Kind
	}{
		{"classified", sentinel, apperr.KindNotFound},
		{"wrapped by fmt", fmt.Errorf("loading: %w", sentinel), apperr.KindNotFound},
		{"unclassified", errors.New("boom"), apperr.KindInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := apperr.KindOf(tt.err); got != tt.want {
				t.Fatalf("KindOf() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWithCauseKeepsSentinelIdentity(t *testing.T) {
	sentinel := apperr.Conflict("things.taken", "already taken")
	cause := errors.New("unique violation")

	err := sentinel.WithCause(cause)

	if !errors.Is(err, sentinel) {
		t.Fatal("copy with cause should still match the sentinel")
	}
	if !errors.Is(err, cause) {
		t.Fatal("cause should be reachable through Unwrap")
	}
	if errors.Unwrap(sentinel) != nil {
		t.Fatal("WithCause must not mutate the sentinel")
	}
}
