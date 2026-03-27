package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/fatihiazmi/ledger-engine/internal/domain/account"
	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
)

// OutboxAppender writes events to both event store and outbox atomically.
type OutboxAppender interface {
	AppendWithOutbox(ctx context.Context, aggregateID, aggregateType string, expectedVersion int64, events []ledger.StoredEvent) error
}

// LedgerService is the CQRS write side (command handler).
// Validates commands, updates domain state, emits events to the event store.
type LedgerService struct {
	accounts     account.Repository
	transactions ledger.TransactionRepository
	eventStore   ledger.EventStore
	outbox       OutboxAppender
	projector    Projector
}

// Projector processes domain events to update read models.
type Projector interface {
	ProjectAccountOpened(ctx context.Context, event ledger.AccountOpenedEvent) error
	ProjectTransactionRecorded(ctx context.Context, event ledger.TransactionRecordedEvent) error
}

func NewLedgerService(
	accounts account.Repository,
	transactions ledger.TransactionRepository,
	eventStore ledger.EventStore,
	outbox OutboxAppender,
	projector Projector,
) *LedgerService {
	return &LedgerService{
		accounts:     accounts,
		transactions: transactions,
		eventStore:   eventStore,
		outbox:       outbox,
		projector:    projector,
	}
}

// OpenAccount creates a new account, persists it, and emits an event.
func (s *LedgerService) OpenAccount(ctx context.Context, cmd OpenAccountCmd) (account.Account, error) {
	accType := account.Asset
	switch cmd.AccountType {
	case "EQUITY":
		accType = account.Equity
	case "LIABILITY":
		accType = account.Liability
	}

	acc, err := account.NewAccountWithType(cmd.Name, accType, cmd.Currency)
	if err != nil {
		return account.Account{}, fmt.Errorf("open account: %w", err)
	}

	if err := s.accounts.Save(ctx, acc); err != nil {
		return account.Account{}, fmt.Errorf("save account: %w", err)
	}

	// Emit event
	event := ledger.AccountOpenedEvent{
		AccountID:   acc.ID().String(),
		Name:        acc.Name(),
		AccountType: string(accType),
		Currency:    string(acc.Currency()),
		Timestamp:   time.Now().UTC(),
	}

	if err := s.appendEvent(ctx, acc.ID().String(), "account", 0, event); err != nil {
		return account.Account{}, fmt.Errorf("emit account opened: %w", err)
	}

	if s.projector != nil {
		s.projector.ProjectAccountOpened(ctx, event)
	}

	return acc, nil
}

// OpenEquityAccount is a convenience for creating equity accounts.
func (s *LedgerService) OpenEquityAccount(ctx context.Context, name string, currency ledger.Currency) (account.Account, error) {
	return s.OpenAccount(ctx, OpenAccountCmd{Name: name, AccountType: "EQUITY", Currency: currency})
}

// Deposit adds funds to an account from a system equity account.
// This is how money enters the system — the equity plumbing is handled automatically.
func (s *LedgerService) Deposit(ctx context.Context, accountID ledger.AccountID, amount int64, currency ledger.Currency, description string) error {
	equityAcc, err := s.getOrCreateSystemEquity(ctx, currency)
	if err != nil {
		return fmt.Errorf("get system equity: %w", err)
	}

	_, err = s.RecordTransaction(ctx, RecordTransactionCmd{
		Entries: []EntryCmd{
			{AccountID: equityAcc.ID(), Amount: amount, Currency: currency, Type: ledger.Debit},
			{AccountID: accountID, Amount: amount, Currency: currency, Type: ledger.Credit},
		},
		Description: description,
	})
	return err
}

func (s *LedgerService) getOrCreateSystemEquity(ctx context.Context, currency ledger.Currency) (account.Account, error) {
	return s.getOrCreateSystemAccount(ctx, fmt.Sprintf("System Equity (%s)", currency), "EQUITY", currency)
}

// suspenseAccountID returns the system suspense account for a currency.
// Suspense accounts hold money in-flight during saga transfers.
func (s *LedgerService) suspenseAccountID(ctx context.Context, currency ledger.Currency) ledger.AccountID {
	acc, err := s.getOrCreateSystemAccount(ctx, fmt.Sprintf("System Suspense (%s)", currency), "LIABILITY", currency)
	if err != nil {
		return ledger.AccountID{} // will fail at transaction validation
	}
	return acc.ID()
}

func (s *LedgerService) getOrCreateSystemAccount(ctx context.Context, name, accType string, currency ledger.Currency) (account.Account, error) {
	all, err := s.accounts.FindAll(ctx)
	if err != nil {
		return account.Account{}, err
	}

	for _, acc := range all {
		if acc.Name() == name {
			return acc, nil
		}
	}

	return s.OpenAccount(ctx, OpenAccountCmd{
		Name:        name,
		AccountType: accType,
		Currency:    currency,
	})
}

// GetAccount retrieves an account by ID (from write model).
func (s *LedgerService) GetAccount(ctx context.Context, id ledger.AccountID) (account.Account, error) {
	return s.accounts.FindByID(ctx, id)
}

// GetBalance retrieves current balance (from write model).
func (s *LedgerService) GetBalance(ctx context.Context, id ledger.AccountID) (ledger.Money, error) {
	acc, err := s.accounts.FindByID(ctx, id)
	if err != nil {
		return ledger.Money{}, fmt.Errorf("get balance: %w", err)
	}
	return acc.Balance(), nil
}

// RecordTransaction validates, records a transaction, updates balances, and emits events.
func (s *LedgerService) RecordTransaction(ctx context.Context, cmd RecordTransactionCmd) (ledger.Transaction, error) {
	params := make([]ledger.EntryParams, len(cmd.Entries))
	for i, e := range cmd.Entries {
		money, err := ledger.NewMoney(e.Amount, e.Currency)
		if err != nil {
			return ledger.Transaction{}, fmt.Errorf("invalid money in entry %d: %w", i, err)
		}
		params[i] = ledger.EntryParams{
			AccountID: e.AccountID,
			Amount:    money,
			Type:      e.Type,
		}
	}

	tx, err := ledger.NewTransaction(params, cmd.Description)
	if err != nil {
		return ledger.Transaction{}, fmt.Errorf("create transaction: %w", err)
	}

	// Update account balances (write model)
	for _, entry := range tx.Entries() {
		acc, err := s.accounts.FindByID(ctx, entry.AccountID())
		if err != nil {
			return ledger.Transaction{}, fmt.Errorf("find account %s: %w", entry.AccountID(), err)
		}

		switch entry.Type() {
		case ledger.Debit:
			acc, err = acc.Debit(entry.Amount())
		case ledger.Credit:
			acc, err = acc.Credit(entry.Amount())
		}
		if err != nil {
			return ledger.Transaction{}, fmt.Errorf("apply %s to account %s: %w", entry.Type(), entry.AccountID(), err)
		}

		if err := s.accounts.Save(ctx, acc); err != nil {
			return ledger.Transaction{}, fmt.Errorf("save account %s: %w", entry.AccountID(), err)
		}
	}

	if err := s.transactions.Save(ctx, tx); err != nil {
		return ledger.Transaction{}, fmt.Errorf("save transaction: %w", err)
	}

	// Emit event
	entryData := make([]ledger.EntryEventData, len(tx.Entries()))
	for i, e := range tx.Entries() {
		entryData[i] = ledger.EntryEventData{
			EntryID:   e.ID().String(),
			AccountID: e.AccountID().String(),
			Amount:    e.Amount().Amount(),
			Currency:  string(e.Amount().Currency()),
			Type:      string(e.Type()),
		}
	}

	event := ledger.TransactionRecordedEvent{
		TransactionID: tx.ID().String(),
		Entries:       entryData,
		Description:   cmd.Description,
		Timestamp:     time.Now().UTC(),
	}

	if err := s.appendEvent(ctx, tx.ID().String(), "transaction", 0, event); err != nil {
		return ledger.Transaction{}, fmt.Errorf("emit transaction recorded: %w", err)
	}

	if s.projector != nil {
		s.projector.ProjectTransactionRecorded(ctx, event)
	}

	return tx, nil
}

func (s *LedgerService) appendEvent(ctx context.Context, aggregateID, aggregateType string, expectedVersion int64, event ledger.DomainEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	stored := ledger.StoredEvent{
		EventType:  event.EventType(),
		Payload:    payload,
		OccurredAt: event.OccurredAt(),
	}

	// Prefer outbox (atomic write to event store + outbox in one transaction)
	if s.outbox != nil {
		return s.outbox.AppendWithOutbox(ctx, aggregateID, aggregateType, expectedVersion, []ledger.StoredEvent{stored})
	}

	// Fallback to direct event store (unit tests, no outbox)
	if s.eventStore != nil {
		return s.eventStore.Append(ctx, aggregateID, aggregateType, expectedVersion, []ledger.StoredEvent{stored})
	}

	return nil
}
