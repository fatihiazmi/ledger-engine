package app_test

import (
	"context"
	"testing"

	"github.com/fatihiazmi/ledger-engine/internal/app"
	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	"github.com/fatihiazmi/ledger-engine/internal/domain/transfer"
	"github.com/fatihiazmi/ledger-engine/internal/infra/inmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func newTestTransferService() (*app.TransferService, *app.LedgerService) {
	ledgerSvc := app.NewLedgerService(
		inmemory.NewAccountRepository(),
		inmemory.NewTransactionRepository(),
		nil, nil,
	)
	transferSvc := app.NewTransferService(ledgerSvc)
	return transferSvc, ledgerSvc
}

func TestTransferSaga_Success(t *testing.T) {
	transferSvc, ledgerSvc := newTestTransferService()
	ctx := context.Background()

	// Setup: create and fund accounts
	source, _ := ledgerSvc.OpenAccount(ctx, app.OpenAccountCmd{Name: "Source", Currency: ledger.USD})
	dest, _ := ledgerSvc.OpenAccount(ctx, app.OpenAccountCmd{Name: "Dest", Currency: ledger.USD})
	ledgerSvc.Deposit(ctx, source.ID(), 100000, ledger.USD, "funding")

	// Execute transfer
	saga, err := transferSvc.Execute(ctx, app.TransferCmd{
		FromAccountID: source.ID(),
		ToAccountID:   dest.ID(),
		Amount:        30000,
		Currency:      ledger.USD,
		Description:   "rent payment",
	})

	require.NoError(t, err)
	assert.Equal(t, transfer.Completed, saga.State())
	assert.Len(t, saga.Steps(), 3) // initiated, debited, completed

	// Verify balances
	s, _ := ledgerSvc.GetAccount(ctx, source.ID())
	d, _ := ledgerSvc.GetAccount(ctx, dest.ID())
	assert.Equal(t, int64(70000), s.Balance().Amount())
	assert.Equal(t, int64(30000), d.Balance().Amount())
}

func TestTransferSaga_InsufficientFunds(t *testing.T) {
	transferSvc, ledgerSvc := newTestTransferService()
	ctx := context.Background()

	source, _ := ledgerSvc.OpenAccount(ctx, app.OpenAccountCmd{Name: "Source", Currency: ledger.USD})
	dest, _ := ledgerSvc.OpenAccount(ctx, app.OpenAccountCmd{Name: "Dest", Currency: ledger.USD})
	ledgerSvc.Deposit(ctx, source.ID(), 10000, ledger.USD, "small deposit")

	// Try to transfer more than available
	saga, err := transferSvc.Execute(ctx, app.TransferCmd{
		FromAccountID: source.ID(),
		ToAccountID:   dest.ID(),
		Amount:        50000,
		Currency:      ledger.USD,
		Description:   "too much",
	})

	assert.Error(t, err)
	assert.Equal(t, transfer.Failed, saga.State())

	// Balance unchanged
	s, _ := ledgerSvc.GetAccount(ctx, source.ID())
	assert.Equal(t, int64(10000), s.Balance().Amount())
}

func TestTransferSaga_SameAccountRejected(t *testing.T) {
	transferSvc, ledgerSvc := newTestTransferService()
	ctx := context.Background()

	acc, _ := ledgerSvc.OpenAccount(ctx, app.OpenAccountCmd{Name: "Self", Currency: ledger.USD})

	_, err := transferSvc.Execute(ctx, app.TransferCmd{
		FromAccountID: acc.ID(),
		ToAccountID:   acc.ID(),
		Amount:        1000,
		Currency:      ledger.USD,
		Description:   "self transfer",
	})

	assert.Error(t, err)
}

func TestTransferSaga_CompensationOnCreditFailure(t *testing.T) {
	transferSvc, ledgerSvc := newTestTransferService()
	ctx := context.Background()

	source, _ := ledgerSvc.OpenAccount(ctx, app.OpenAccountCmd{Name: "Source", Currency: ledger.USD})
	ledgerSvc.Deposit(ctx, source.ID(), 50000, ledger.USD, "funding")

	// Transfer to non-existent account — debit will succeed, credit will fail, compensation kicks in
	badAccountID := ledger.GenerateAccountID()
	saga, err := transferSvc.Execute(ctx, app.TransferCmd{
		FromAccountID: source.ID(),
		ToAccountID:   badAccountID,
		Amount:        20000,
		Currency:      ledger.USD,
		Description:   "to nowhere",
	})

	assert.Error(t, err)
	assert.Equal(t, transfer.Compensated, saga.State())
	assert.Contains(t, saga.FailReason(), "credit failed")

	// Source balance restored after compensation
	s, _ := ledgerSvc.GetAccount(ctx, source.ID())
	assert.Equal(t, int64(50000), s.Balance().Amount(), "balance should be restored after compensation")
}

// === PBT: Money conservation across random transfers ===

func TestTransferPBT_MoneyConserved(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		transferSvc, ledgerSvc := newTestTransferService()
		ctx := context.Background()

		acc1, _ := ledgerSvc.OpenAccount(ctx, app.OpenAccountCmd{Name: "A", Currency: ledger.USD})
		acc2, _ := ledgerSvc.OpenAccount(ctx, app.OpenAccountCmd{Name: "B", Currency: ledger.USD})

		seed := rapid.Int64Range(10000, 1_000_000_00).Draw(t, "seed")
		ledgerSvc.Deposit(ctx, acc1.ID(), seed, ledger.USD, "seed")

		numTransfers := rapid.IntRange(1, 10).Draw(t, "num")
		for i := 0; i < numTransfers; i++ {
			a, _ := ledgerSvc.GetAccount(ctx, acc1.ID())
			if a.Balance().Amount() <= 0 {
				break
			}
			amt := rapid.Int64Range(1, a.Balance().Amount()).Draw(t, "amt")

			transferSvc.Execute(ctx, app.TransferCmd{
				FromAccountID: acc1.ID(),
				ToAccountID:   acc2.ID(),
				Amount:        amt,
				Currency:      ledger.USD,
				Description:   "transfer",
			})
		}

		// Money conserved across both accounts
		a1, _ := ledgerSvc.GetAccount(ctx, acc1.ID())
		a2, _ := ledgerSvc.GetAccount(ctx, acc2.ID())
		assert.Equal(t, seed, a1.Balance().Amount()+a2.Balance().Amount(),
			"total money must be conserved")
	})
}
