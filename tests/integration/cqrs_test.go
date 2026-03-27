//go:build integration

package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatihiazmi/ledger-engine/internal/app"
	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	"github.com/fatihiazmi/ledger-engine/internal/infra/inmemory"
	pgstore "github.com/fatihiazmi/ledger-engine/internal/infra/postgres"
	"github.com/fatihiazmi/ledger-engine/internal/projection"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupCQRS(t *testing.T) (*pgxpool.Pool, *app.LedgerService, *projection.PostgresQueryService) {
	t.Helper()
	ctx := context.Background()

	migrationsPath, err := filepath.Abs("../../migrations")
	require.NoError(t, err)

	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("ledger_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithInitScripts(
			filepath.Join(migrationsPath, "000001_create_event_store.up.sql"),
			filepath.Join(migrationsPath, "000002_create_read_models.up.sql"),
		),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	eventStore := pgstore.NewEventStore(pool)
	projector := projection.NewPostgresProjector(pool)
	queryService := projection.NewPostgresQueryService(pool)

	svc := app.NewLedgerService(
		inmemory.NewAccountRepository(),
		inmemory.NewTransactionRepository(),
		eventStore,
		nil, // no outbox in CQRS test
		projector,
	)

	return pool, svc, queryService
}

func TestCQRS_WriteAndReadSides(t *testing.T) {
	_, svc, queries := setupCQRS(t)
	ctx := context.Background()

	t.Run("open account appears in read model", func(t *testing.T) {
		acc, err := svc.OpenAccount(ctx, app.OpenAccountCmd{
			Name:     "Nik's Checking",
			Currency: ledger.USD,
		})
		require.NoError(t, err)

		view, err := queries.GetAccountBalance(ctx, acc.ID().String())
		require.NoError(t, err)
		assert.Equal(t, "Nik's Checking", view.Name)
		assert.Equal(t, "USD", view.Currency)
		assert.Equal(t, int64(0), view.Balance)
		assert.Equal(t, "ACTIVE", view.Status)
	})

	t.Run("transaction updates read model balances", func(t *testing.T) {
		source, _ := svc.OpenAccount(ctx, app.OpenAccountCmd{Name: "Source", Currency: ledger.USD})
		dest, _ := svc.OpenAccount(ctx, app.OpenAccountCmd{Name: "Dest", Currency: ledger.USD})
		equity, _ := svc.OpenEquityAccount(ctx, "Equity", ledger.USD)

		// Fund source from equity
		_, err := svc.RecordTransaction(ctx, app.RecordTransactionCmd{
			Entries: []app.EntryCmd{
				{AccountID: equity.ID(), Amount: 50000, Currency: ledger.USD, Type: ledger.Debit},
				{AccountID: source.ID(), Amount: 50000, Currency: ledger.USD, Type: ledger.Credit},
			},
			Description: "initial funding",
		})
		require.NoError(t, err)

		// Transfer from source to dest
		_, err = svc.RecordTransaction(ctx, app.RecordTransactionCmd{
			Entries: []app.EntryCmd{
				{AccountID: source.ID(), Amount: 20000, Currency: ledger.USD, Type: ledger.Debit},
				{AccountID: dest.ID(), Amount: 20000, Currency: ledger.USD, Type: ledger.Credit},
			},
			Description: "payment for services",
		})
		require.NoError(t, err)

		// Query read model
		sourceView, err := queries.GetAccountBalance(ctx, source.ID().String())
		require.NoError(t, err)
		assert.Equal(t, int64(30000), sourceView.Balance)

		destView, err := queries.GetAccountBalance(ctx, dest.ID().String())
		require.NoError(t, err)
		assert.Equal(t, int64(20000), destView.Balance)
	})

	t.Run("transaction history is recorded", func(t *testing.T) {
		acc, _ := svc.OpenAccount(ctx, app.OpenAccountCmd{Name: "History Test", Currency: ledger.USD})
		equity, _ := svc.OpenEquityAccount(ctx, "Equity2", ledger.USD)

		// Fund and then make 3 transactions
		svc.RecordTransaction(ctx, app.RecordTransactionCmd{
			Entries: []app.EntryCmd{
				{AccountID: equity.ID(), Amount: 100000, Currency: ledger.USD, Type: ledger.Debit},
				{AccountID: acc.ID(), Amount: 100000, Currency: ledger.USD, Type: ledger.Credit},
			},
			Description: "funding",
		})

		for i := 0; i < 3; i++ {
			svc.RecordTransaction(ctx, app.RecordTransactionCmd{
				Entries: []app.EntryCmd{
					{AccountID: acc.ID(), Amount: 10000, Currency: ledger.USD, Type: ledger.Debit},
					{AccountID: equity.ID(), Amount: 10000, Currency: ledger.USD, Type: ledger.Credit},
				},
				Description: "withdrawal",
			})
		}

		history, err := queries.GetTransactionHistory(ctx, acc.ID().String(), 50, 0)
		require.NoError(t, err)
		assert.Len(t, history, 4) // 1 funding + 3 withdrawals

		// Most recent first (DESC order)
		assert.Equal(t, "DEBIT", history[0].EntryType)
		assert.Equal(t, int64(10000), history[0].Amount)
	})

	t.Run("list all accounts", func(t *testing.T) {
		accounts, err := queries.ListAccounts(ctx)
		require.NoError(t, err)
		assert.True(t, len(accounts) >= 3, "should have at least 3 accounts")
	})

	t.Run("read model is independent from write model", func(t *testing.T) {
		acc, _ := svc.OpenAccount(ctx, app.OpenAccountCmd{Name: "Independence", Currency: ledger.USD})

		// Write model (in-memory) has the account
		writeAcc, err := svc.GetAccount(ctx, acc.ID())
		require.NoError(t, err)
		assert.Equal(t, "Independence", writeAcc.Name())

		// Read model (Postgres) also has the account — independently
		readView, err := queries.GetAccountBalance(ctx, acc.ID().String())
		require.NoError(t, err)
		assert.Equal(t, "Independence", readView.Name)
	})
}
