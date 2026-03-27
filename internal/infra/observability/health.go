package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// HealthChecker answers the 3 questions from DB Reliability Engineering:
// 1. Is the data safe? (Postgres reachable)
// 2. Is the service up? (end-to-end check)
// 3. Are consumers in pain? (latency)
type HealthChecker struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

func NewHealthChecker(db *pgxpool.Pool, redis *redis.Client) *HealthChecker {
	return &HealthChecker{db: db, redis: redis}
}

type healthResponse struct {
	Status   string            `json:"status"`
	Checks   map[string]check  `json:"checks"`
	Uptime   string            `json:"uptime"`
}

type check struct {
	Status  string `json:"status"`
	Latency string `json:"latency,omitempty"`
	Error   string `json:"error,omitempty"`
}

var startTime = time.Now()

func (h *HealthChecker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		checks := make(map[string]check)
		allHealthy := true

		// Check 1: Is the data safe? (Postgres)
		checks["postgres"] = h.checkPostgres(ctx)
		if checks["postgres"].Status != "ok" {
			allHealthy = false
		}

		// Check 2: Is the service up? (Redis)
		checks["redis"] = h.checkRedis(ctx)
		if checks["redis"].Status != "ok" {
			allHealthy = false
		}

		// Check 3: Outbox backlog (are consumers in pain?)
		checks["outbox"] = h.checkOutboxBacklog(ctx)

		status := "healthy"
		httpStatus := http.StatusOK
		if !allHealthy {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		resp := healthResponse{
			Status: status,
			Checks: checks,
			Uptime: time.Since(startTime).Round(time.Second).String(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(httpStatus)
		json.NewEncoder(w).Encode(resp)
	}
}

func (h *HealthChecker) checkPostgres(ctx context.Context) check {
	start := time.Now()
	err := h.db.Ping(ctx)
	latency := time.Since(start)

	if err != nil {
		return check{Status: "error", Latency: latency.String(), Error: err.Error()}
	}
	return check{Status: "ok", Latency: latency.String()}
}

func (h *HealthChecker) checkRedis(ctx context.Context) check {
	start := time.Now()
	err := h.redis.Ping(ctx).Err()
	latency := time.Since(start)

	if err != nil {
		return check{Status: "error", Latency: latency.String(), Error: err.Error()}
	}
	return check{Status: "ok", Latency: latency.String()}
}

func (h *HealthChecker) checkOutboxBacklog(ctx context.Context) check {
	var count int
	err := h.db.QueryRow(ctx, "SELECT COUNT(*) FROM outbox WHERE published = FALSE").Scan(&count)
	if err != nil {
		return check{Status: "error", Error: err.Error()}
	}

	status := "ok"
	if count > 100 {
		status = "warning"
	}
	if count > 1000 {
		status = "critical"
	}

	return check{Status: status, Latency: fmt.Sprintf("%d pending", count)}
}
