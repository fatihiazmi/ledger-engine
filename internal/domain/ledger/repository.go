package ledger

import "context"

// TransactionRepository defines the port for transaction persistence.
type TransactionRepository interface {
	Save(ctx context.Context, tx Transaction) error
	FindByID(ctx context.Context, id TransactionID) (Transaction, error)
	FindByAccountID(ctx context.Context, accountID AccountID) ([]Transaction, error)
}
