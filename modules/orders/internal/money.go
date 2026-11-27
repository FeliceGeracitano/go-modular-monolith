package core

import (
	"math"

	"github.com/FeliceGeracitano/go-modular-monolith/pkg/apperr"
)

var (
	ErrInvalidCurrency  = apperr.Invalid("orders.invalid_currency", "currency must be a 3-letter ISO 4217 code")
	ErrCurrencyMismatch = apperr.Invalid("orders.currency_mismatch", "amounts must share the same currency")
	ErrOutOfRange       = apperr.Invalid("orders.out_of_range", "amount is out of range")
)

type Money struct {
	Amount   int64
	Currency string
}

func NewMoney(amount int64, currency string) (Money, error) {
	if !validCurrency(currency) {
		return Money{}, ErrInvalidCurrency
	}
	return Money{Amount: amount, Currency: currency}, nil
}

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
