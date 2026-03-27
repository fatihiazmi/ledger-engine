package app

import "github.com/fatihiazmi/ledger-engine/internal/domain/ledger"

// OpenAccountCmd is a command to create a new account.
type OpenAccountCmd struct {
	Name        string
	AccountType string // "ASSET", "LIABILITY", "EQUITY"
	Currency    ledger.Currency
}

// RecordTransactionCmd is a command to record a balanced transaction.
type RecordTransactionCmd struct {
	Entries     []EntryCmd
	Description string
}

// EntryCmd is the command input for a single entry in a transaction.
type EntryCmd struct {
	AccountID ledger.AccountID
	Amount    int64
	Currency  ledger.Currency
	Type      ledger.EntryType
}
