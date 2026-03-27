package ledger

import "errors"

// Currency represents an ISO 4217 currency code.
// Domain primitive — validated at creation, immutable.
type Currency string

const (
	USD Currency = "USD"
	MYR Currency = "MYR"
	SGD Currency = "SGD"
	EUR Currency = "EUR"
	GBP Currency = "GBP"
)

var validCurrencies = map[Currency]bool{
	USD: true, MYR: true, SGD: true, EUR: true, GBP: true,
}

var (
	ErrInvalidCurrency   = errors.New("invalid currency")
	ErrCurrencyMismatch  = errors.New("cannot operate on different currencies")
)

// Money is a domain primitive representing a monetary amount in the smallest unit (cents).
// Immutable. If it exists, it's valid.
type Money struct {
	amount   int64
	currency Currency
}

// NewMoney creates a validated Money value object.
// Amount is in smallest currency unit (cents). Negative allowed for debits.
func NewMoney(amount int64, currency Currency) (Money, error) {
	if !validCurrencies[currency] {
		return Money{}, ErrInvalidCurrency
	}
	return Money{amount: amount, currency: currency}, nil
}

func (m Money) Amount() int64      { return m.amount }
func (m Money) Currency() Currency { return m.currency }
func (m Money) IsZero() bool       { return m.amount == 0 }

func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	return Money{amount: m.amount + other.amount, currency: m.currency}, nil
}

func (m Money) Subtract(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	return Money{amount: m.amount - other.amount, currency: m.currency}, nil
}

func (m Money) Negate() Money {
	return Money{amount: -m.amount, currency: m.currency}
}

func (m Money) Equals(other Money) bool {
	return m.amount == other.amount && m.currency == other.currency
}
