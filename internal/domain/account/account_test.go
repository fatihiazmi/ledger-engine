package account_test

import (
	"testing"

	"github.com/fatihiazmi/ledger-engine/internal/domain/account"
	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestNewAccount(t *testing.T) {
	t.Run("creates active account with zero balance", func(t *testing.T) {
		acc, err := account.NewAccount("Nik's Checking", ledger.USD)
		require.NoError(t, err)
		assert.False(t, acc.ID().IsZero())
		assert.Equal(t, "Nik's Checking", acc.Name())
		assert.Equal(t, ledger.USD, acc.Currency())
		assert.True(t, acc.Balance().IsZero())
		assert.Equal(t, account.Active, acc.Status())
	})

	t.Run("rejects empty name", func(t *testing.T) {
		_, err := account.NewAccount("", ledger.USD)
		assert.ErrorIs(t, err, account.ErrEmptyName)
	})

	t.Run("rejects invalid currency", func(t *testing.T) {
		_, err := account.NewAccount("Test", ledger.Currency("FAKE"))
		assert.Error(t, err)
	})
}

func TestAccountDebit(t *testing.T) {
	t.Run("debits reduce balance", func(t *testing.T) {
		acc := mustAccount("Checking", ledger.USD)
		acc = mustCredit(acc, 5000) // seed with $50

		updated, err := acc.Debit(mustMoney(2000, ledger.USD))
		require.NoError(t, err)
		assert.Equal(t, int64(3000), updated.Balance().Amount())
	})

	t.Run("rejects debit exceeding balance", func(t *testing.T) {
		acc := mustAccount("Checking", ledger.USD)
		acc = mustCredit(acc, 1000)

		_, err := acc.Debit(mustMoney(2000, ledger.USD))
		assert.ErrorIs(t, err, account.ErrInsufficientFunds)
	})

	t.Run("rejects debit on suspended account", func(t *testing.T) {
		acc := mustAccount("Checking", ledger.USD)
		acc = mustCredit(acc, 5000)
		acc = acc.Suspend()

		_, err := acc.Debit(mustMoney(1000, ledger.USD))
		assert.ErrorIs(t, err, account.ErrAccountNotActive)
	})

	t.Run("rejects debit with wrong currency", func(t *testing.T) {
		acc := mustAccount("Checking", ledger.USD)
		acc = mustCredit(acc, 5000)

		_, err := acc.Debit(mustMoney(1000, ledger.MYR))
		assert.ErrorIs(t, err, ledger.ErrCurrencyMismatch)
	})

	t.Run("rejects zero debit", func(t *testing.T) {
		acc := mustAccount("Checking", ledger.USD)
		_, err := acc.Debit(mustMoney(0, ledger.USD))
		assert.ErrorIs(t, err, account.ErrZeroAmount)
	})
}

func TestAccountCredit(t *testing.T) {
	t.Run("credits increase balance", func(t *testing.T) {
		acc := mustAccount("Savings", ledger.USD)

		updated, err := acc.Credit(mustMoney(3000, ledger.USD))
		require.NoError(t, err)
		assert.Equal(t, int64(3000), updated.Balance().Amount())
	})

	t.Run("rejects credit on closed account", func(t *testing.T) {
		acc := mustAccount("Old Account", ledger.USD)
		acc = acc.Close()

		_, err := acc.Credit(mustMoney(1000, ledger.USD))
		assert.ErrorIs(t, err, account.ErrAccountNotActive)
	})
}

func TestAccountLifecycle(t *testing.T) {
	t.Run("active -> suspended -> active", func(t *testing.T) {
		acc := mustAccount("Test", ledger.USD)
		assert.Equal(t, account.Active, acc.Status())

		acc = acc.Suspend()
		assert.Equal(t, account.Suspended, acc.Status())

		acc = acc.Reactivate()
		assert.Equal(t, account.Active, acc.Status())
	})

	t.Run("active -> closed is terminal", func(t *testing.T) {
		acc := mustAccount("Test", ledger.USD)
		acc = acc.Close()
		assert.Equal(t, account.Closed, acc.Status())

		// cannot reactivate a closed account
		acc2 := acc.Reactivate()
		assert.Equal(t, account.Closed, acc2.Status())
	})
}

// === Property-Based Tests ===

func TestAccountPBT_CreditThenDebitConservesMoney(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		creditAmt := rapid.Int64Range(1, 1_000_000_00).Draw(t, "credit")
		debitAmt := rapid.Int64Range(1, creditAmt).Draw(t, "debit")

		acc := mustAccount("PBT", ledger.USD)
		acc, _ = acc.Credit(mustMoney(creditAmt, ledger.USD))
		acc, _ = acc.Debit(mustMoney(debitAmt, ledger.USD))

		expected := creditAmt - debitAmt
		assert.Equal(t, expected, acc.Balance().Amount(),
			"balance should equal total credits minus total debits")
	})
}

func TestAccountPBT_SequentialCreditsAccumulate(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		numOps := rapid.IntRange(1, 20).Draw(t, "numOps")
		acc := mustAccount("PBT", ledger.USD)

		var totalCredited int64
		for i := 0; i < numOps; i++ {
			amt := rapid.Int64Range(1, 100_000).Draw(t, "amount")
			acc, _ = acc.Credit(mustMoney(amt, ledger.USD))
			totalCredited += amt
		}

		assert.Equal(t, totalCredited, acc.Balance().Amount(),
			"balance should equal sum of all credits")
	})
}

func TestAccountPBT_DebitNeverExceedsBalance(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		balance := rapid.Int64Range(1, 1_000_000_00).Draw(t, "balance")
		debitAmt := rapid.Int64Range(balance+1, balance+1_000_000).Draw(t, "debit")

		acc := mustAccount("PBT", ledger.USD)
		acc, _ = acc.Credit(mustMoney(balance, ledger.USD))

		_, err := acc.Debit(mustMoney(debitAmt, ledger.USD))
		assert.ErrorIs(t, err, account.ErrInsufficientFunds,
			"debit exceeding balance must always be rejected")
	})
}

// helpers
func mustAccount(name string, currency ledger.Currency) account.Account {
	acc, err := account.NewAccount(name, currency)
	if err != nil {
		panic(err)
	}
	return acc
}

func mustCredit(acc account.Account, amount int64) account.Account {
	updated, err := acc.Credit(mustMoney(amount, ledger.USD))
	if err != nil {
		panic(err)
	}
	return updated
}

func mustMoney(amount int64, currency ledger.Currency) ledger.Money {
	m, err := ledger.NewMoney(amount, currency)
	if err != nil {
		panic(err)
	}
	return m
}
