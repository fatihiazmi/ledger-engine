package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fatihiazmi/ledger-engine/internal/domain/account"
	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAccountNotFound = errors.New("account not found")

type AccountRepository struct {
	pool *pgxpool.Pool
}

func NewAccountRepository(pool *pgxpool.Pool) *AccountRepository {
	return &AccountRepository{pool: pool}
}

func (r *AccountRepository) Save(ctx context.Context, acc account.Account) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO accounts (account_id, name, account_type, currency, balance, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (account_id) DO UPDATE SET
		   balance = $5, status = $6, updated_at = $8`,
		acc.ID().String(), acc.Name(), string(acc.Type()), string(acc.Currency()),
		acc.Balance().Amount(), string(acc.Status()), acc.CreatedAt(), acc.UpdatedAt(),
	)
	return err
}

func (r *AccountRepository) FindByID(ctx context.Context, id ledger.AccountID) (account.Account, error) {
	var (
		accID, name, accType, currency, status string
		balance                                int64
		createdAt, updatedAt                   time.Time
	)

	err := r.pool.QueryRow(ctx,
		`SELECT account_id, name, account_type, currency, balance, status, created_at, updated_at
		 FROM accounts WHERE account_id = $1`,
		id.String(),
	).Scan(&accID, &name, &accType, &currency, &balance, &status, &createdAt, &updatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return account.Account{}, fmt.Errorf("find account %s: %w", id, ErrAccountNotFound)
	}
	if err != nil {
		return account.Account{}, fmt.Errorf("query account: %w", err)
	}

	return account.Reconstitute(accID, name, accType, currency, balance, status, createdAt, updatedAt)
}

func (r *AccountRepository) FindAll(ctx context.Context) ([]account.Account, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT account_id, name, account_type, currency, balance, status, created_at, updated_at
		 FROM accounts ORDER BY name`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []account.Account
	for rows.Next() {
		var (
			accID, name, accType, currency, status string
			balance                                int64
			createdAt, updatedAt                   time.Time
		)
		if err := rows.Scan(&accID, &name, &accType, &currency, &balance, &status, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		acc, err := account.Reconstitute(accID, name, accType, currency, balance, status, createdAt, updatedAt)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, acc)
	}
	return accounts, rows.Err()
}
