package core_test

import (
	"errors"
	"testing"
	"time"

	core "github.com/FeliceGeracitano/go-modular-monolith/modules/customers/internal"
)

func TestNewCustomer(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	tests := []struct {
		name      string
		inName    string
		inEmail   string
		wantErr   error
		wantEmail string
	}{
		{"valid", "Ada Lovelace", "ada@example.com", nil, "ada@example.com"},
		{"normalises input", "  Ada  ", " Ada@Example.COM ", nil, "ada@example.com"},
		{"empty name", "   ", "ada@example.com", core.ErrInvalidName, ""},
		{"malformed email", "Ada", "not-an-email", core.ErrInvalidEmail, ""},
		{"display-name email", "Ada", "Ada <ada@example.com>", core.ErrInvalidEmail, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := core.NewCustomer("id-1", tt.inName, tt.inEmail, now)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if err == nil && got.Email != tt.wantEmail {
				t.Fatalf("email = %q, want %q", got.Email, tt.wantEmail)
			}
		})
	}
}
