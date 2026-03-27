package inmemory

import (
	"context"
	"errors"
	"sync"

	"github.com/fatihiazmi/ledger-engine/internal/domain/account"
	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
)

var ErrAccountNotFound = errors.New("account not found")

// AccountRepository is an in-memory fake implementing account.Repository.
type AccountRepository struct {
	mu       sync.RWMutex
	accounts map[string]account.Account
}

func NewAccountRepository() *AccountRepository {
	return &AccountRepository{
		accounts: make(map[string]account.Account),
	}
}

func (r *AccountRepository) Save(_ context.Context, acc account.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accounts[acc.ID().String()] = acc
	return nil
}

func (r *AccountRepository) FindByID(_ context.Context, id ledger.AccountID) (account.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	acc, ok := r.accounts[id.String()]
	if !ok {
		return account.Account{}, ErrAccountNotFound
	}
	return acc, nil
}

func (r *AccountRepository) FindAll(_ context.Context) ([]account.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]account.Account, 0, len(r.accounts))
	for _, acc := range r.accounts {
		result = append(result, acc)
	}
	return result, nil
}
