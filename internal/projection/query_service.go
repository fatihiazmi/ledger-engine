package projection

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAccountNotFound = errors.New("account not found in read model")

// PostgresQueryService implements QueryService using the read model tables.
type PostgresQueryService struct {
	pool *pgxpool.Pool
}

func NewPostgresQueryService(pool *pgxpool.Pool) *PostgresQueryService {
	return &PostgresQueryService{pool: pool}
}

func (q *PostgresQueryService) GetAccountBalance(ctx context.Context, accountID string) (*AccountView, error) {
	var v AccountView
	err := q.pool.QueryRow(ctx,
		`SELECT account_id, name, account_type, currency, balance, status, version, updated_at
		 FROM account_balances WHERE account_id = $1`,
		accountID,
	).Scan(&v.AccountID, &v.Name, &v.AccountType, &v.Currency, &v.Balance, &v.Status, &v.Version, &v.UpdatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query account balance: %w", err)
	}
	return &v, nil
}

func (q *PostgresQueryService) ListAccounts(ctx context.Context) ([]AccountView, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT account_id, name, account_type, currency, balance, status, version, updated_at
		 FROM account_balances ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []AccountView
	for rows.Next() {
		var v AccountView
		if err := rows.Scan(&v.AccountID, &v.Name, &v.AccountType, &v.Currency, &v.Balance, &v.Status, &v.Version, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		accounts = append(accounts, v)
	}
	return accounts, rows.Err()
}

func (q *PostgresQueryService) GetTransactionHistory(ctx context.Context, accountID string, limit, offset int) ([]TransactionHistoryEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := q.pool.Query(ctx,
		`SELECT id, transaction_id, account_id, entry_type, amount, currency, description, balance_after, created_at
		 FROM transaction_history
		 WHERE account_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		accountID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	var entries []TransactionHistoryEntry
	for rows.Next() {
		var e TransactionHistoryEntry
		if err := rows.Scan(&e.ID, &e.TransactionID, &e.AccountID, &e.EntryType, &e.Amount, &e.Currency, &e.Description, &e.BalanceAfter, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan history entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
