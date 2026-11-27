package core_test

import (
	"errors"
	"math"
	"testing"

	core "github.com/FeliceGeracitano/go-modular-monolith/modules/orders/internal"
)

func TestNewMoneyValidatesCurrency(t *testing.T) {
	for _, code := range []string{"", "eur", "EURO", "E1R"} {
		if _, err := core.NewMoney(100, code); !errors.Is(err, core.ErrInvalidCurrency) {
			t.Errorf("NewMoney(100, %q) error = %v, want ErrInvalidCurrency", code, err)
		}
	}
	if _, err := core.NewMoney(100, "EUR"); err != nil {
		t.Fatalf("NewMoney(100, EUR) unexpected error: %v", err)
	}
}

func TestMoneyArithmetic(t *testing.T) {
	a, _ := core.NewMoney(250, "EUR")
	b, _ := core.NewMoney(100, "EUR")
	usd, _ := core.NewMoney(100, "USD")

	sum, err := a.Add(b)
	if err != nil || sum.Amount != 350 {
		t.Fatalf("Add = %+v, %v; want 350 EUR", sum, err)
	}
	if _, err := a.Add(usd); !errors.Is(err, core.ErrCurrencyMismatch) {
		t.Fatalf("Add across currencies error = %v, want ErrCurrencyMismatch", err)
	}
	if got, err := a.Mul(3); err != nil || got.Amount != 750 || got.Currency != "EUR" {
		t.Fatalf("Mul = %+v, %v; want 750 EUR", got, err)
	}
	huge, _ := core.NewMoney(math.MaxInt64, "EUR")
	one, _ := core.NewMoney(1, "EUR")
	if _, err := huge.Add(one); !errors.Is(err, core.ErrOutOfRange) {
		t.Fatalf("Add overflow error = %v, want ErrOutOfRange", err)
	}
	if _, err := huge.Mul(2); !errors.Is(err, core.ErrOutOfRange) {
		t.Fatalf("Mul overflow error = %v, want ErrOutOfRange", err)
	}
}
