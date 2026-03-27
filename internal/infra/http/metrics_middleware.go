package http

import (
	"net/http"
	"time"

	"github.com/fatihiazmi/ledger-engine/internal/infra/observability"
	"github.com/go-chi/chi/v5"
)

// MetricsMiddleware records request duration and count per endpoint.
func MetricsMiddleware(metrics *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, statusCode: 200}

			next.ServeHTTP(recorder, r)

			// Use chi route pattern (e.g., "/api/v1/accounts/{accountID}") not actual path
			routePattern := chi.RouteContext(r.Context()).RoutePattern()
			if routePattern == "" {
				routePattern = r.URL.Path
			}

			metrics.RecordRequest(r.Context(), r.Method, routePattern, recorder.statusCode, time.Since(start))
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}
