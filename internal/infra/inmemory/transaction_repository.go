package inmemory

import (
	"context"
	"errors"
	"sync"

	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
)

var ErrTransactionNotFound = errors.New("transaction not found")

// TransactionRepository is an in-memory fake implementing ledger.TransactionRepository.
type TransactionRepository struct {
	mu           sync.RWMutex
	transactions map[string]ledger.Transaction
}

func NewTransactionRepository() *TransactionRepository {
	return &TransactionRepository{
		transactions: make(map[string]ledger.Transaction),
	}
}

func (r *TransactionRepository) Save(_ context.Context, tx ledger.Transaction) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.transactions[tx.ID().String()] = tx
	return nil
}

func (r *TransactionRepository) FindByID(_ context.Context, id ledger.TransactionID) (ledger.Transaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tx, ok := r.transactions[id.String()]
	if !ok {
		return ledger.Transaction{}, ErrTransactionNotFound
	}
	return tx, nil
}

func (r *TransactionRepository) FindByAccountID(_ context.Context, accountID ledger.AccountID) ([]ledger.Transaction, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []ledger.Transaction
	for _, tx := range r.transactions {
		for _, entry := range tx.Entries() {
			if entry.AccountID().Equals(accountID) {
				result = append(result, tx)
				break
			}
		}
	}
	return result, nil
}
