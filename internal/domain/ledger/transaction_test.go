package ledger_test

import (
	"testing"

	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestNewTransaction(t *testing.T) {
	acc1 := ledger.GenerateAccountID()
	acc2 := ledger.GenerateAccountID()

	t.Run("creates balanced transaction with debit and credit", func(t *testing.T) {
		entries := []ledger.EntryParams{
			{AccountID: acc1, Amount: mustMoney(1000, ledger.USD), Type: ledger.Debit},
			{AccountID: acc2, Amount: mustMoney(1000, ledger.USD), Type: ledger.Credit},
		}

		tx, err := ledger.NewTransaction(entries, "test transfer")
		require.NoError(t, err)
		assert.Equal(t, 2, len(tx.Entries()))
		assert.Equal(t, "test transfer", tx.Description())
		assert.False(t, tx.ID().IsZero())
	})

	t.Run("rejects unbalanced transaction", func(t *testing.T) {
		entries := []ledger.EntryParams{
			{AccountID: acc1, Amount: mustMoney(1000, ledger.USD), Type: ledger.Debit},
			{AccountID: acc2, Amount: mustMoney(500, ledger.USD), Type: ledger.Credit},
		}

		_, err := ledger.NewTransaction(entries, "bad transfer")
		assert.ErrorIs(t, err, ledger.ErrUnbalancedTransaction)
	})

	t.Run("rejects empty entries", func(t *testing.T) {
		_, err := ledger.NewTransaction(nil, "empty")
		assert.ErrorIs(t, err, ledger.ErrNoEntries)
	})

	t.Run("rejects single entry", func(t *testing.T) {
		entries := []ledger.EntryParams{
			{AccountID: acc1, Amount: mustMoney(1000, ledger.USD), Type: ledger.Debit},
		}

		_, err := ledger.NewTransaction(entries, "single")
		assert.ErrorIs(t, err, ledger.ErrInsufficientEntries)
	})

	t.Run("rejects mixed currencies", func(t *testing.T) {
		entries := []ledger.EntryParams{
			{AccountID: acc1, Amount: mustMoney(1000, ledger.USD), Type: ledger.Debit},
			{AccountID: acc2, Amount: mustMoney(1000, ledger.MYR), Type: ledger.Credit},
		}

		_, err := ledger.NewTransaction(entries, "mixed")
		assert.ErrorIs(t, err, ledger.ErrCurrencyMismatch)
	})

	t.Run("rejects zero amount entry", func(t *testing.T) {
		entries := []ledger.EntryParams{
			{AccountID: acc1, Amount: mustMoney(0, ledger.USD), Type: ledger.Debit},
			{AccountID: acc2, Amount: mustMoney(0, ledger.USD), Type: ledger.Credit},
		}

		_, err := ledger.NewTransaction(entries, "zero")
		assert.ErrorIs(t, err, ledger.ErrZeroAmount)
	})

	t.Run("supports multi-leg transactions", func(t *testing.T) {
		acc3 := ledger.GenerateAccountID()
		entries := []ledger.EntryParams{
			{AccountID: acc1, Amount: mustMoney(1000, ledger.USD), Type: ledger.Debit},
			{AccountID: acc2, Amount: mustMoney(600, ledger.USD), Type: ledger.Credit},
			{AccountID: acc3, Amount: mustMoney(400, ledger.USD), Type: ledger.Credit},
		}

		tx, err := ledger.NewTransaction(entries, "split payment")
		require.NoError(t, err)
		assert.Equal(t, 3, len(tx.Entries()))
	})
}

// === Property-Based Tests: Double-Entry Invariants ===

func TestTransactionPBT_CreditsAlwaysEqualDebits(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random balanced transaction
		amount := rapid.Int64Range(1, 1_000_000_00).Draw(t, "amount")
		acc1 := ledger.GenerateAccountID()
		acc2 := ledger.GenerateAccountID()

		entries := []ledger.EntryParams{
			{AccountID: acc1, Amount: mustMoney(amount, ledger.USD), Type: ledger.Debit},
			{AccountID: acc2, Amount: mustMoney(amount, ledger.USD), Type: ledger.Credit},
		}

		tx, err := ledger.NewTransaction(entries, "pbt-transfer")
		require.NoError(t, err)

		// Invariant: sum of debits == sum of credits
		var totalDebits, totalCredits int64
		for _, e := range tx.Entries() {
			if e.Type() == ledger.Debit {
				totalDebits += e.Amount().Amount()
			} else {
				totalCredits += e.Amount().Amount()
			}
		}

		assert.Equal(t, totalDebits, totalCredits, "debits must equal credits")
	})
}

func TestTransactionPBT_UnbalancedAlwaysRejected(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		debitAmt := rapid.Int64Range(1, 1_000_000_00).Draw(t, "debit")
		creditAmt := rapid.Int64Range(1, 1_000_000_00).Draw(t, "credit")

		if debitAmt == creditAmt {
			t.Skip("balanced — not testing this case")
		}

		acc1 := ledger.GenerateAccountID()
		acc2 := ledger.GenerateAccountID()

		entries := []ledger.EntryParams{
			{AccountID: acc1, Amount: mustMoney(debitAmt, ledger.USD), Type: ledger.Debit},
			{AccountID: acc2, Amount: mustMoney(creditAmt, ledger.USD), Type: ledger.Credit},
		}

		_, err := ledger.NewTransaction(entries, "unbalanced")
		assert.ErrorIs(t, err, ledger.ErrUnbalancedTransaction, "unbalanced transactions must always be rejected")
	})
}

func TestTransactionPBT_MultiLegBalanced(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate N credit amounts that sum to the debit
		numCredits := rapid.IntRange(2, 5).Draw(t, "numCredits")
		totalAmount := rapid.Int64Range(int64(numCredits), 1_000_000_00).Draw(t, "total")

		acc1 := ledger.GenerateAccountID()
		entries := []ledger.EntryParams{
			{AccountID: acc1, Amount: mustMoney(totalAmount, ledger.USD), Type: ledger.Debit},
		}

		// Split into N credits
		remaining := totalAmount
		for i := 0; i < numCredits; i++ {
			var creditAmt int64
			if i == numCredits-1 {
				creditAmt = remaining
			} else {
				creditAmt = rapid.Int64Range(1, remaining-int64(numCredits-1-i)).Draw(t, "credit")
				remaining -= creditAmt
			}
			entries = append(entries, ledger.EntryParams{
				AccountID: ledger.GenerateAccountID(),
				Amount:    mustMoney(creditAmt, ledger.USD),
				Type:      ledger.Credit,
			})
		}

		tx, err := ledger.NewTransaction(entries, "multi-leg")
		require.NoError(t, err)

		var totalDebits, totalCredits int64
		for _, e := range tx.Entries() {
			if e.Type() == ledger.Debit {
				totalDebits += e.Amount().Amount()
			} else {
				totalCredits += e.Amount().Amount()
			}
		}
		assert.Equal(t, totalDebits, totalCredits)
		assert.Equal(t, numCredits+1, len(tx.Entries()))
	})
}

// helper
func mustMoney(amount int64, currency ledger.Currency) ledger.Money {
	m, err := ledger.NewMoney(amount, currency)
	if err != nil {
		panic(err)
	}
	return m
}
