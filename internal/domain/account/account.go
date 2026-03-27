package account

import (
	"errors"
	"time"

	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
)

var (
	ErrEmptyName         = errors.New("account name must not be empty")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrAccountNotActive  = errors.New("account is not active")
	ErrZeroAmount        = errors.New("amount must not be zero")
)

// AccountType determines balance constraints.
// Asset/Expense accounts cannot go negative. Liability/Equity/Revenue accounts can.
type AccountType string

const (
	Asset     AccountType = "ASSET"     // e.g., checking, savings — balance >= 0
	Liability AccountType = "LIABILITY" // e.g., loans, payables — can go negative
	Equity    AccountType = "EQUITY"    // e.g., owner's capital, retained earnings — can go negative
)

// Status represents the lifecycle state of an account.
// Entity state object pattern (Secure by Design — Ch. 7).
type Status string

const (
	Active    Status = "ACTIVE"
	Suspended Status = "SUSPENDED"
	Closed    Status = "CLOSED"
)

// Account is an entity representing a financial account.
// Returns new instances on mutation (partially immutable pattern).
type Account struct {
	id          ledger.AccountID
	name        string
	accountType AccountType
	currency    ledger.Currency
	balance     ledger.Money
	status      Status
	createdAt   time.Time
	updatedAt   time.Time
}

// NewAccount creates a validated Asset account with zero balance and Active status.
func NewAccount(name string, currency ledger.Currency) (Account, error) {
	return NewAccountWithType(name, Asset, currency)
}

// NewAccountWithType creates a validated account of a specific type.
func NewAccountWithType(name string, accountType AccountType, currency ledger.Currency) (Account, error) {
	if name == "" {
		return Account{}, ErrEmptyName
	}

	balance, err := ledger.NewMoney(0, currency)
	if err != nil {
		return Account{}, err
	}

	now := time.Now().UTC()
	return Account{
		id:          ledger.GenerateAccountID(),
		name:        name,
		accountType: accountType,
		currency:    currency,
		balance:     balance,
		status:      Active,
		createdAt:   now,
		updatedAt:   now,
	}, nil
}

// Reconstitute rebuilds an Account from persisted state (e.g., database row).
// Skips validation — the data was already validated when first created.
func Reconstitute(id, name, accountType, currency string, balance int64, status string, createdAt, updatedAt time.Time) (Account, error) {
	accID, err := ledger.NewAccountID(id)
	if err != nil {
		return Account{}, err
	}
	bal, err := ledger.NewMoney(balance, ledger.Currency(currency))
	if err != nil {
		return Account{}, err
	}
	return Account{
		id:          accID,
		name:        name,
		accountType: AccountType(accountType),
		currency:    ledger.Currency(currency),
		balance:     bal,
		status:      Status(status),
		createdAt:   createdAt,
		updatedAt:   updatedAt,
	}, nil
}

func (a Account) ID() ledger.AccountID      { return a.id }
func (a Account) Name() string              { return a.name }
func (a Account) Type() AccountType         { return a.accountType }
func (a Account) Currency() ledger.Currency { return a.currency }
func (a Account) Balance() ledger.Money     { return a.balance }
func (a Account) Status() Status            { return a.status }
func (a Account) CreatedAt() time.Time      { return a.createdAt }
func (a Account) UpdatedAt() time.Time      { return a.updatedAt }

// AllowsNegativeBalance returns true for liability and equity accounts.
func (a Account) AllowsNegativeBalance() bool {
	return a.accountType == Liability || a.accountType == Equity
}

// Credit increases the account balance. Returns a new Account.
func (a Account) Credit(amount ledger.Money) (Account, error) {
	if err := a.validateOperation(amount); err != nil {
		return Account{}, err
	}

	newBalance, err := a.balance.Add(amount)
	if err != nil {
		return Account{}, err
	}

	a.balance = newBalance
	a.updatedAt = time.Now().UTC()
	return a, nil
}

// Debit decreases the account balance. Returns a new Account.
// Rejects if insufficient funds.
func (a Account) Debit(amount ledger.Money) (Account, error) {
	if err := a.validateOperation(amount); err != nil {
		return Account{}, err
	}

	newBalance, err := a.balance.Subtract(amount)
	if err != nil {
		return Account{}, err
	}

	if newBalance.Amount() < 0 && !a.AllowsNegativeBalance() {
		return Account{}, ErrInsufficientFunds
	}

	a.balance = newBalance
	a.updatedAt = time.Now().UTC()
	return a, nil
}

// Suspend moves account to Suspended state. Only Active accounts can be suspended.
func (a Account) Suspend() Account {
	if a.status == Active {
		a.status = Suspended
		a.updatedAt = time.Now().UTC()
	}
	return a
}

// Reactivate moves account back to Active. Only Suspended accounts can be reactivated.
func (a Account) Reactivate() Account {
	if a.status == Suspended {
		a.status = Active
		a.updatedAt = time.Now().UTC()
	}
	return a
}

// Close moves account to Closed state (terminal). Cannot be reopened.
func (a Account) Close() Account {
	if a.status != Closed {
		a.status = Closed
		a.updatedAt = time.Now().UTC()
	}
	return a
}

func (a Account) validateOperation(amount ledger.Money) error {
	if a.status != Active {
		return ErrAccountNotActive
	}
	if amount.IsZero() {
		return ErrZeroAmount
	}
	if amount.Currency() != a.currency {
		return ledger.ErrCurrencyMismatch
	}
	return nil
}
