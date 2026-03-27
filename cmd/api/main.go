package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/fatihiazmi/ledger-engine/internal/app"
	httpapi "github.com/fatihiazmi/ledger-engine/internal/infra/http"
	// inmemory kept for transaction repo (events are source of truth for transactions)
	"github.com/fatihiazmi/ledger-engine/internal/infra/inmemory"
	pgstore "github.com/fatihiazmi/ledger-engine/internal/infra/postgres"
	"github.com/fatihiazmi/ledger-engine/internal/projection"
	"github.com/fatihiazmi/ledger-engine/web"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://ledger:ledger@localhost:5432/ledger_db?sslmode=disable"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("ping database: %v", err)
	}
	log.Println("Connected to Postgres")

	eventStore := pgstore.NewEventStore(pool)
	projector := projection.NewPostgresProjector(pool)
	queryService := projection.NewPostgresQueryService(pool)

	svc := app.NewLedgerService(
		pgstore.NewAccountRepository(pool),
		inmemory.NewTransactionRepository(),
		eventStore,
		projector,
	)

	handler := httpapi.NewHandler(svc, queryService)

	// Serve embedded static files
	staticFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		log.Fatalf("static files: %v", err)
	}

	router := httpapi.NewRouter(handler, staticFS)

	fmt.Printf("\n  Ledger Engine running at http://localhost:%s\n\n", port)
	fmt.Println("  API:       /api/v1/accounts, /api/v1/transactions")
	fmt.Println("  Dashboard: /")
	fmt.Println()

	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
