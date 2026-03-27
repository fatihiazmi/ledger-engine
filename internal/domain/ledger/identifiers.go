package ledger

import (
	"errors"

	"github.com/google/uuid"
)

var ErrInvalidID = errors.New("invalid identifier: must be valid UUID")

// AccountID is a domain primitive for account identification.
// Immutable, validated at creation. Uses UUID v7 (time-ordered).
type AccountID struct {
	value uuid.UUID
}

func NewAccountID(id string) (AccountID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return AccountID{}, ErrInvalidID
	}
	return AccountID{value: parsed}, nil
}

func GenerateAccountID() AccountID {
	return AccountID{value: uuid.Must(uuid.NewV7())}
}

func (id AccountID) String() string  { return id.value.String() }
func (id AccountID) IsZero() bool    { return id.value == uuid.Nil }
func (id AccountID) Equals(o AccountID) bool { return id.value == o.value }

// TransactionID is a domain primitive for transaction identification.
type TransactionID struct {
	value uuid.UUID
}

func NewTransactionID(id string) (TransactionID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return TransactionID{}, ErrInvalidID
	}
	return TransactionID{value: parsed}, nil
}

func GenerateTransactionID() TransactionID {
	return TransactionID{value: uuid.Must(uuid.NewV7())}
}

func (id TransactionID) String() string  { return id.value.String() }
func (id TransactionID) IsZero() bool    { return id.value == uuid.Nil }
func (id TransactionID) Equals(o TransactionID) bool { return id.value == o.value }

// EntryID is a domain primitive for ledger entry identification.
type EntryID struct {
	value uuid.UUID
}

func NewEntryID(id string) (EntryID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return EntryID{}, ErrInvalidID
	}
	return EntryID{value: parsed}, nil
}

func GenerateEntryID() EntryID {
	return EntryID{value: uuid.Must(uuid.NewV7())}
}

func (id EntryID) String() string { return id.value.String() }
func (id EntryID) IsZero() bool   { return id.value == uuid.Nil }
