package money_test

import (
	"errors"
	"math"
	"testing"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/money"
)

func TestNewValidatesCurrency(t *testing.T) {
	for _, code := range []string{"", "eur", "EURO", "E1R"} {
		if _, err := money.New(100, code); !errors.Is(err, money.ErrInvalidCurrency) {
			t.Errorf("New(100, %q) error = %v, want ErrInvalidCurrency", code, err)
		}
	}
	if _, err := money.New(100, "EUR"); err != nil {
		t.Fatalf("New(100, EUR) unexpected error: %v", err)
	}
}

func TestAddAndMul(t *testing.T) {
	a, _ := money.New(250, "EUR")
	b, _ := money.New(100, "EUR")
	usd, _ := money.New(100, "USD")

	sum, err := a.Add(b)
	if err != nil || sum.Amount != 350 {
		t.Fatalf("Add = %+v, %v; want 350 EUR", sum, err)
	}
	if _, err := a.Add(usd); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("Add across currencies error = %v, want ErrCurrencyMismatch", err)
	}
	if got, err := a.Mul(3); err != nil || got.Amount != 750 || got.Currency != "EUR" {
		t.Fatalf("Mul = %+v, %v; want 750 EUR", got, err)
	}
}

func TestArithmeticRejectsOverflow(t *testing.T) {
	huge, _ := money.New(math.MaxInt64, "EUR")
	one, _ := money.New(1, "EUR")

	if _, err := huge.Add(one); !errors.Is(err, money.ErrOutOfRange) {
		t.Fatalf("Add overflow error = %v, want ErrOutOfRange", err)
	}
	if _, err := huge.Mul(2); !errors.Is(err, money.ErrOutOfRange) {
		t.Fatalf("Mul overflow error = %v, want ErrOutOfRange", err)
	}
	if got, err := huge.Mul(1); err != nil || got != huge {
		t.Fatalf("Mul(1) = %+v, %v; want the same amount", got, err)
	}
}
