package ledger

import (
	"errors"
	"time"
)

var (
	ErrNoEntries              = errors.New("transaction requires at least one entry")
	ErrInsufficientEntries    = errors.New("transaction requires at least two entries")
	ErrUnbalancedTransaction  = errors.New("transaction is not balanced: total debits must equal total credits")
	ErrZeroAmount             = errors.New("entry amount must not be zero")
)

// Transaction is an aggregate root representing a double-entry bookkeeping transaction.
// Immutable after creation. Contains 2+ entries where debits == credits.
// If it exists, it's balanced.
type Transaction struct {
	id          TransactionID
	entries     []Entry
	description string
	createdAt   time.Time
}

// NewTransaction creates a validated, balanced transaction.
// Enforces the double-entry invariant at construction time (Secure by Design).
func NewTransaction(params []EntryParams, description string) (Transaction, error) {
	if len(params) == 0 {
		return Transaction{}, ErrNoEntries
	}
	if len(params) < 2 {
		return Transaction{}, ErrInsufficientEntries
	}

	// Validate: no zero amounts
	for _, p := range params {
		if p.Amount.IsZero() {
			return Transaction{}, ErrZeroAmount
		}
	}

	// Validate: all same currency
	currency := params[0].Amount.Currency()
	for _, p := range params[1:] {
		if p.Amount.Currency() != currency {
			return Transaction{}, ErrCurrencyMismatch
		}
	}

	// Validate: debits == credits (the core invariant)
	var totalDebits, totalCredits int64
	for _, p := range params {
		switch p.Type {
		case Debit:
			totalDebits += p.Amount.Amount()
		case Credit:
			totalCredits += p.Amount.Amount()
		}
	}
	if totalDebits != totalCredits {
		return Transaction{}, ErrUnbalancedTransaction
	}

	// All validations passed — construct entries
	now := time.Now().UTC()
	entries := make([]Entry, len(params))
	for i, p := range params {
		entries[i] = newEntry(p, now)
	}

	return Transaction{
		id:          GenerateTransactionID(),
		entries:     entries,
		description: description,
		createdAt:   now,
	}, nil
}

func (t Transaction) ID() TransactionID  { return t.id }
func (t Transaction) Description() string { return t.description }
func (t Transaction) CreatedAt() time.Time { return t.createdAt }

// Entries returns a defensive copy of the entries slice.
func (t Transaction) Entries() []Entry {
	cp := make([]Entry, len(t.entries))
	copy(cp, t.entries)
	return cp
}
