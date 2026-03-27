package app_test

import (
	"context"
	"testing"

	"github.com/fatihiazmi/ledger-engine/internal/app"
	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	"github.com/fatihiazmi/ledger-engine/internal/infra/inmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func newTestService() *app.LedgerService {
	return app.NewLedgerService(
		inmemory.NewAccountRepository(),
		inmemory.NewTransactionRepository(),
		nil, // no event store for unit tests
		nil, // no outbox for unit tests
		nil, // no projector for unit tests
	)
}

func TestOpenAccount(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	t.Run("opens new account", func(t *testing.T) {
		acc, err := svc.OpenAccount(ctx, app.OpenAccountCmd{Name: "Nik's Checking", Currency: ledger.USD})
		require.NoError(t, err)
		assert.Equal(t, "Nik's Checking", acc.Name())
		assert.True(t, acc.Balance().IsZero())
	})

	t.Run("rejects empty name", func(t *testing.T) {
		_, err := svc.OpenAccount(ctx, app.OpenAccountCmd{Name: "", Currency: ledger.USD})
		assert.Error(t, err)
	})
}

func TestRecordTransaction(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	acc1, _ := svc.OpenAccount(ctx, app.OpenAccountCmd{Name: "Source", Currency: ledger.USD})
	acc2, _ := svc.OpenAccount(ctx, app.OpenAccountCmd{Name: "Destination", Currency: ledger.USD})
	equity, _ := svc.OpenEquityAccount(ctx, "Owner's Equity", ledger.USD)

	// Seed source with funds (debit equity, credit asset — money enters the system)
	_, err := svc.RecordTransaction(ctx, app.RecordTransactionCmd{
		Entries: []app.EntryCmd{
			{AccountID: equity.ID(), Amount: 10000, Currency: ledger.USD, Type: ledger.Debit},
			{AccountID: acc1.ID(), Amount: 10000, Currency: ledger.USD, Type: ledger.Credit},
		},
		Description: "initial funding from equity",
	})
	require.NoError(t, err)

	t.Run("records balanced transaction and updates balances", func(t *testing.T) {
		tx, err := svc.RecordTransaction(ctx, app.RecordTransactionCmd{
			Entries: []app.EntryCmd{
				{AccountID: acc1.ID(), Amount: 3000, Currency: ledger.USD, Type: ledger.Debit},
				{AccountID: acc2.ID(), Amount: 3000, Currency: ledger.USD, Type: ledger.Credit},
			},
			Description: "payment for services",
		})
		require.NoError(t, err)
		assert.False(t, tx.ID().IsZero())

		// Verify balances updated
		source, _ := svc.GetAccount(ctx, acc1.ID())
		dest, _ := svc.GetAccount(ctx, acc2.ID())
		assert.Equal(t, int64(7000), source.Balance().Amount())
		assert.Equal(t, int64(3000), dest.Balance().Amount())
	})

	t.Run("rejects unbalanced transaction", func(t *testing.T) {
		_, err := svc.RecordTransaction(ctx, app.RecordTransactionCmd{
			Entries: []app.EntryCmd{
				{AccountID: acc1.ID(), Amount: 1000, Currency: ledger.USD, Type: ledger.Debit},
				{AccountID: acc2.ID(), Amount: 500, Currency: ledger.USD, Type: ledger.Credit},
			},
			Description: "bad transaction",
		})
		assert.Error(t, err)
	})

	t.Run("rejects debit exceeding balance", func(t *testing.T) {
		_, err := svc.RecordTransaction(ctx, app.RecordTransactionCmd{
			Entries: []app.EntryCmd{
				{AccountID: acc1.ID(), Amount: 999999, Currency: ledger.USD, Type: ledger.Debit},
				{AccountID: acc2.ID(), Amount: 999999, Currency: ledger.USD, Type: ledger.Credit},
			},
			Description: "overdraft attempt",
		})
		assert.Error(t, err)
	})
}

func TestGetAccountBalance(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	acc, _ := svc.OpenAccount(ctx, app.OpenAccountCmd{Name: "Test", Currency: ledger.USD})

	balance, err := svc.GetBalance(ctx, acc.ID())
	require.NoError(t, err)
	assert.True(t, balance.IsZero())
}

// === Property-Based Tests: End-to-End Money Conservation ===

func TestPBT_MoneyConservation(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		svc := newTestService()
		ctx := context.Background()

		acc1, _ := svc.OpenAccount(ctx, app.OpenAccountCmd{Name: "A", Currency: ledger.USD})
		acc2, _ := svc.OpenAccount(ctx, app.OpenAccountCmd{Name: "B", Currency: ledger.USD})
		equity, _ := svc.OpenEquityAccount(ctx, "Equity", ledger.USD)

		// Seed acc1 with funds from equity
		seedAmount := rapid.Int64Range(10000, 1_000_000_00).Draw(t, "seed")
		svc.RecordTransaction(ctx, app.RecordTransactionCmd{
			Entries: []app.EntryCmd{
				{AccountID: equity.ID(), Amount: seedAmount, Currency: ledger.USD, Type: ledger.Debit},
				{AccountID: acc1.ID(), Amount: seedAmount, Currency: ledger.USD, Type: ledger.Credit},
			},
			Description: "seed",
		})

		// Perform random transfers between acc1 and acc2
		numTransfers := rapid.IntRange(1, 10).Draw(t, "numTransfers")
		for i := 0; i < numTransfers; i++ {
			a1, _ := svc.GetAccount(ctx, acc1.ID())
			if a1.Balance().Amount() <= 0 {
				break
			}
			transferAmt := rapid.Int64Range(1, a1.Balance().Amount()).Draw(t, "transfer")

			svc.RecordTransaction(ctx, app.RecordTransactionCmd{
				Entries: []app.EntryCmd{
					{AccountID: acc1.ID(), Amount: transferAmt, Currency: ledger.USD, Type: ledger.Debit},
					{AccountID: acc2.ID(), Amount: transferAmt, Currency: ledger.USD, Type: ledger.Credit},
				},
				Description: "transfer",
			})
		}

		// Invariant: asset balances == seed amount
		final1, _ := svc.GetAccount(ctx, acc1.ID())
		final2, _ := svc.GetAccount(ctx, acc2.ID())
		actualTotal := final1.Balance().Amount() + final2.Balance().Amount()

		assert.Equal(t, seedAmount, actualTotal,
			"money must be conserved across all transfers")
	})
}
