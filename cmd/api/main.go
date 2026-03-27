package main

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/fatihiazmi/ledger-engine/internal/app"
	httpapi "github.com/fatihiazmi/ledger-engine/internal/infra/http"
	"github.com/fatihiazmi/ledger-engine/internal/infra/inmemory"
	"github.com/fatihiazmi/ledger-engine/internal/infra/observability"
	pgstore "github.com/fatihiazmi/ledger-engine/internal/infra/postgres"
	"github.com/fatihiazmi/ledger-engine/internal/infra/publisher"
	redisstore "github.com/fatihiazmi/ledger-engine/internal/infra/redis"
	"github.com/fatihiazmi/ledger-engine/internal/projection"
	"github.com/fatihiazmi/ledger-engine/web"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Structured logging
	logger := observability.SetupLogger()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://ledger:ledger@localhost:5432/ledger_db?sslmode=disable"
	}

	redisAddr := os.Getenv("REDIS_URL")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	// Postgres
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		logger.Error("connect to database", slog.Any("error", err))
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Error("ping database", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("connected to Postgres")

	// Redis
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Error("connect to redis", slog.Any("error", err))
		os.Exit(1)
	}
	defer rdb.Close()
	logger.Info("connected to Redis")

	// Metrics
	metrics, _, err := observability.SetupMetrics()
	if err != nil {
		logger.Error("setup metrics", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("metrics initialized (Prometheus)")

	// Wire CQRS + Outbox
	eventStore := pgstore.NewEventStore(pool)
	outboxWriter := pgstore.NewOutboxWriter(pool)
	projector := projection.NewPostgresProjector(pool)
	queryService := projection.NewPostgresQueryService(pool)
	idempotencyStore := redisstore.NewIdempotencyStore(rdb)
	healthChecker := observability.NewHealthChecker(pool, rdb)

	svc := app.NewLedgerService(
		pgstore.NewAccountRepository(pool),
		inmemory.NewTransactionRepository(),
		eventStore,
		outboxWriter,
		projector,
	)

	// Outbox worker
	logPublisher := publisher.NewLogPublisher()
	outboxWorker := app.NewOutboxWorker(outboxWriter, logPublisher)
	go outboxWorker.Start(ctx)

	transferSvc := app.NewTransferService(svc)
	handler := httpapi.NewHandler(svc, transferSvc, queryService)

	staticFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		logger.Error("static files", slog.Any("error", err))
		os.Exit(1)
	}

	router := httpapi.NewRouter(httpapi.RouterDeps{
		Handler:          handler,
		StaticFS:         staticFS,
		IdempotencyStore: idempotencyStore,
		Metrics:          metrics,
		HealthChecker:    healthChecker,
	})

	fmt.Printf("\n  Ledger Engine running at http://localhost:%s\n\n", port)
	fmt.Println("  Dashboard:    /")
	fmt.Println("  API:          /api/v1/*")
	fmt.Println("  Health:       /health")
	fmt.Println("  Metrics:      /metrics")
	fmt.Println()

	server := &http.Server{Addr: ":" + port, Handler: router}

	go func() {
		<-ctx.Done()
		logger.Info("shutting down")
		server.Shutdown(context.Background())
	}()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		logger.Error("server error", slog.Any("error", err))
		os.Exit(1)
	}
}
