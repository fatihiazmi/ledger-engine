package app

import (
	"context"
	"fmt"

	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	"github.com/fatihiazmi/ledger-engine/internal/domain/transfer"
)

// TransferCmd is the command to initiate a money transfer.
type TransferCmd struct {
	FromAccountID ledger.AccountID
	ToAccountID   ledger.AccountID
	Amount        int64
	Currency      ledger.Currency
	Description   string
}

// TransferService orchestrates the transfer saga.
// Implements the Saga Orchestration pattern — a central coordinator
// that drives the steps and handles compensation on failure.
type TransferService struct {
	ledgerSvc *LedgerService
}

func NewTransferService(ledgerSvc *LedgerService) *TransferService {
	return &TransferService{ledgerSvc: ledgerSvc}
}

// Execute runs the transfer saga: debit source → credit destination.
// If credit fails, automatically compensates by reversing the debit.
func (s *TransferService) Execute(ctx context.Context, cmd TransferCmd) (*transfer.Transfer, error) {
	money, err := ledger.NewMoney(cmd.Amount, cmd.Currency)
	if err != nil {
		return nil, fmt.Errorf("invalid amount: %w", err)
	}

	// Step 0: Create saga
	saga, err := transfer.NewTransfer(cmd.FromAccountID, cmd.ToAccountID, money, cmd.Description)
	if err != nil {
		return nil, fmt.Errorf("create transfer: %w", err)
	}

	// Step 1: Debit source account
	_, err = s.ledgerSvc.RecordTransaction(ctx, RecordTransactionCmd{
		Entries: []EntryCmd{
			{AccountID: cmd.FromAccountID, Amount: cmd.Amount, Currency: cmd.Currency, Type: ledger.Debit},
			{AccountID: s.ledgerSvc.suspenseAccountID(ctx, cmd.Currency), Amount: cmd.Amount, Currency: cmd.Currency, Type: ledger.Credit},
		},
		Description: fmt.Sprintf("Transfer out: %s", cmd.Description),
	})
	if err != nil {
		saga.MarkFailed(fmt.Sprintf("debit failed: %v", err))
		return &saga, fmt.Errorf("saga step 1 (debit source): %w", err)
	}

	if err := saga.MarkDebited(); err != nil {
		return &saga, err
	}

	// Step 2: Credit destination account
	_, err = s.ledgerSvc.RecordTransaction(ctx, RecordTransactionCmd{
		Entries: []EntryCmd{
			{AccountID: s.ledgerSvc.suspenseAccountID(ctx, cmd.Currency), Amount: cmd.Amount, Currency: cmd.Currency, Type: ledger.Debit},
			{AccountID: cmd.ToAccountID, Amount: cmd.Amount, Currency: cmd.Currency, Type: ledger.Credit},
		},
		Description: fmt.Sprintf("Transfer in: %s", cmd.Description),
	})
	if err != nil {
		// Compensation: reverse the debit
		saga.MarkFailed(fmt.Sprintf("credit failed: %v", err))

		_, compErr := s.ledgerSvc.RecordTransaction(ctx, RecordTransactionCmd{
			Entries: []EntryCmd{
				{AccountID: s.ledgerSvc.suspenseAccountID(ctx, cmd.Currency), Amount: cmd.Amount, Currency: cmd.Currency, Type: ledger.Debit},
				{AccountID: cmd.FromAccountID, Amount: cmd.Amount, Currency: cmd.Currency, Type: ledger.Credit},
			},
			Description: fmt.Sprintf("Compensation: reverse debit for failed transfer: %s", cmd.Description),
		})
		if compErr != nil {
			return &saga, fmt.Errorf("CRITICAL: compensation failed after credit failure: credit_err=%v, comp_err=%v", err, compErr)
		}

		saga.MarkCompensated()
		return &saga, fmt.Errorf("transfer failed and compensated: %w", err)
	}

	if err := saga.MarkCompleted(); err != nil {
		return &saga, err
	}

	return &saga, nil
}
