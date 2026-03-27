package projection

import (
	"context"
	"fmt"

	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresProjector implements app.Projector by writing to read model tables.
// This is the CQRS read-side event handler.
type PostgresProjector struct {
	pool *pgxpool.Pool
}

func NewPostgresProjector(pool *pgxpool.Pool) *PostgresProjector {
	return &PostgresProjector{pool: pool}
}

// ProjectAccountOpened inserts a new account into the read model.
func (p *PostgresProjector) ProjectAccountOpened(ctx context.Context, event ledger.AccountOpenedEvent) error {
	_, err := p.pool.Exec(ctx,
		`INSERT INTO account_balances (account_id, name, account_type, currency, balance, status, version, updated_at)
		 VALUES ($1, $2, $3, $4, 0, 'ACTIVE', 1, $5)
		 ON CONFLICT (account_id) DO NOTHING`,
		event.AccountID, event.Name, event.AccountType, event.Currency, event.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("project account opened: %w", err)
	}
	return nil
}

// ProjectTransactionRecorded updates balances and inserts history entries.
func (p *PostgresProjector) ProjectTransactionRecorded(ctx context.Context, event ledger.TransactionRecordedEvent) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, entry := range event.Entries {
		// Update balance in read model
		var delta int64
		if entry.Type == "CREDIT" {
			delta = entry.Amount
		} else {
			delta = -entry.Amount
		}

		_, err := tx.Exec(ctx,
			`UPDATE account_balances
			 SET balance = balance + $1, version = version + 1, updated_at = $2
			 WHERE account_id = $3`,
			delta, event.Timestamp, entry.AccountID,
		)
		if err != nil {
			return fmt.Errorf("update balance for %s: %w", entry.AccountID, err)
		}

		// Get balance after update for history
		var balanceAfter int64
		err = tx.QueryRow(ctx,
			`SELECT balance FROM account_balances WHERE account_id = $1`,
			entry.AccountID,
		).Scan(&balanceAfter)
		if err != nil {
			return fmt.Errorf("get balance after for %s: %w", entry.AccountID, err)
		}

		// Insert transaction history entry
		_, err = tx.Exec(ctx,
			`INSERT INTO transaction_history (transaction_id, account_id, entry_type, amount, currency, description, balance_after, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			event.TransactionID, entry.AccountID, entry.Type, entry.Amount, entry.Currency,
			event.Description, balanceAfter, event.Timestamp,
		)
		if err != nil {
			return fmt.Errorf("insert history for %s: %w", entry.AccountID, err)
		}
	}

	return tx.Commit(ctx)
}
