package http

import (
	"io/fs"
	"net/http"

	redisstore "github.com/fatihiazmi/ledger-engine/internal/infra/redis"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func NewRouter(h *Handler, staticFS fs.FS, idempotencyStore *redisstore.IdempotencyStore) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
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

	// Idempotency middleware for write endpoints
	if idempotencyStore != nil {
		r.Use(IdempotencyMiddleware(idempotencyStore))
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/accounts", h.ListAccounts)
		r.Post("/accounts", h.OpenAccount)
		r.Get("/accounts/{accountID}", h.GetBalance)
		r.Get("/accounts/{accountID}/transactions", h.GetTransactionHistory)
		r.Post("/transactions", h.RecordTransaction)
		r.Post("/deposit", h.Deposit)
	})

	if staticFS != nil {
		fileServer := http.FileServerFS(staticFS)
		r.Handle("/*", fileServer)
	}

	return r
}
