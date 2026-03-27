package projection

import "time"

// AccountView is the read model for account balance queries.
type AccountView struct {
	AccountID   string    `json:"account_id"`
	Name        string    `json:"name"`
	AccountType string    `json:"account_type"`
	Currency    string    `json:"currency"`
	Balance     int64     `json:"balance"`
	Status      string    `json:"status"`
	Version     int64     `json:"version"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TransactionHistoryEntry is the read model for transaction history queries.
type TransactionHistoryEntry struct {
	ID            int64     `json:"id"`
	TransactionID string    `json:"transaction_id"`
	AccountID     string    `json:"account_id"`
	EntryType     string    `json:"entry_type"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	Description   string    `json:"description"`
	BalanceAfter  int64     `json:"balance_after"`
	CreatedAt     time.Time `json:"created_at"`
}

// QueryService defines the read side of CQRS.
type QueryService interface {
	GetAccountBalance(accountID string) (*AccountView, error)
	ListAccounts() ([]AccountView, error)
	GetTransactionHistory(accountID string, limit, offset int) ([]TransactionHistoryEntry, error)
}
