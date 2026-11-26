// Package money is a minimal value object for monetary amounts.
//
// It is shared because every module that handles money agrees on these
// rules: integer minor units (cents, never floats), an explicit currency,
// and arithmetic that fails instead of silently overflowing or mixing
// currencies. A module whose amounts mean something different (signed
// refunds, sub-cent prices) should define its own type rather than loosen
// this one for everybody.
package money

import (
	"math"

	"github.com/FeliceGeracitano/go-modular-monolith/internal/kernel/apperr"
)

// Errors returned by this package.
var (
	ErrInvalidCurrency  = apperr.Invalid("money.invalid_currency", "currency must be a 3-letter ISO 4217 code")
	ErrCurrencyMismatch = apperr.Invalid("money.currency_mismatch", "amounts must share the same currency")
	ErrOutOfRange       = apperr.Invalid("money.out_of_range", "amount is out of range")
)

// Money is an amount in minor units of a currency. Build values with New so
// the currency is validated.
type Money struct {
	Amount   int64
	Currency string
}

// New validates the currency code and returns the amount.
func New(amount int64, currency string) (Money, error) {
	if !validCurrency(currency) {
		return Money{}, ErrInvalidCurrency
	}
	return Money{Amount: amount, Currency: currency}, nil
}

// Add returns m + other. Both must share a currency, and the sum must fit.
func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, ErrCurrencyMismatch
	}
	if (other.Amount > 0 && m.Amount > math.MaxInt64-other.Amount) ||
		(other.Amount < 0 && m.Amount < math.MinInt64-other.Amount) {
		return Money{}, ErrOutOfRange
	}
	return Money{Amount: m.Amount + other.Amount, Currency: m.Currency}, nil
}

// Mul returns m multiplied by a quantity. The product must fit.
func (m Money) Mul(quantity int64) (Money, error) {
	product := m.Amount * quantity
	if m.Amount != 0 && (product/m.Amount != quantity || (m.Amount == -1 && quantity == math.MinInt64)) {
		return Money{}, ErrOutOfRange
	}
	return Money{Amount: product, Currency: m.Currency}, nil
}

func validCurrency(code string) bool {
	if len(code) != 3 {
		return false
	}
	for _, r := range code {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
