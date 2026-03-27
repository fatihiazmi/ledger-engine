package http

import (
	"io/fs"
	"net/http"

	"github.com/fatihiazmi/ledger-engine/internal/infra/observability"
	redisstore "github.com/fatihiazmi/ledger-engine/internal/infra/redis"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type RouterDeps struct {
	Handler          *Handler
	StaticFS         fs.FS
	IdempotencyStore *redisstore.IdempotencyStore
	Metrics          *observability.Metrics
	HealthChecker    *observability.HealthChecker
}

func NewRouter(deps RouterDeps) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "Idempotency-Key"},
		ExposedHeaders:   []string{"Content-Length", "Idempotency-Replayed"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Observability middleware
	if deps.Metrics != nil {
		r.Use(MetricsMiddleware(deps.Metrics))
	}
	r.Use(middleware.Logger)

	// Idempotency middleware
	if deps.IdempotencyStore != nil {
		r.Use(IdempotencyMiddleware(deps.IdempotencyStore))
	}

	// Health & metrics endpoints
	r.Get("/health", deps.HealthChecker.Handler())
	r.Handle("/metrics", promhttp.Handler())

	// API
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/accounts", deps.Handler.ListAccounts)
		r.Post("/accounts", deps.Handler.OpenAccount)
		r.Get("/accounts/{accountID}", deps.Handler.GetBalance)
		r.Get("/accounts/{accountID}/transactions", deps.Handler.GetTransactionHistory)
		r.Post("/transactions", deps.Handler.RecordTransaction)
		r.Post("/deposit", deps.Handler.Deposit)
		r.Post("/transfers", deps.Handler.Transfer)
	})

	// Frontend
	if deps.StaticFS != nil {
		fileServer := http.FileServerFS(deps.StaticFS)
		r.Handle("/*", fileServer)
	}

	return r
}
