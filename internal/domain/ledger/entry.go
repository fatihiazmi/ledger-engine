package ledger

import "time"

// EntryType represents the direction of a ledger entry.
type EntryType string

const (
	Debit  EntryType = "DEBIT"
	Credit EntryType = "CREDIT"
)

// EntryParams is the input for creating an Entry within a Transaction.
// Not a domain object itself — just a parameter struct.
type EntryParams struct {
	AccountID AccountID
	Amount    Money
	Type      EntryType
}

// Entry is a value object representing one leg of a double-entry transaction.
// Immutable. Always belongs to a Transaction.
type Entry struct {
	id        EntryID
	accountID AccountID
	amount    Money
	entryType EntryType
	createdAt time.Time
}

func newEntry(params EntryParams, now time.Time) Entry {
	return Entry{
		id:        GenerateEntryID(),
		accountID: params.AccountID,
		amount:    params.Amount,
		entryType: params.Type,
		createdAt: now,
	}
}

func (e Entry) ID() EntryID        { return e.id }
func (e Entry) AccountID() AccountID { return e.accountID }
func (e Entry) Amount() Money       { return e.amount }
func (e Entry) Type() EntryType     { return e.entryType }
func (e Entry) CreatedAt() time.Time { return e.createdAt }
