package http

import (
	"bytes"
	"net/http"

	redisstore "github.com/fatihiazmi/ledger-engine/internal/infra/redis"
)

// IdempotencyMiddleware ensures write requests with the same Idempotency-Key
// are only processed once. Subsequent identical requests return the cached response.
func IdempotencyMiddleware(store *redisstore.IdempotencyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only apply to write operations
			if r.Method != http.MethodPost && r.Method != http.MethodPut {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				// No key provided — process normally (optional enforcement)
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()

			// Check if already processed
			cached, err := store.Check(ctx, key)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "idempotency check failed"})
				return
			}
			if cached != nil {
				// Return cached response
				status, _ := store.GetStatus(ctx, key)
				if status == 0 {
					status = http.StatusOK
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Idempotency-Replayed", "true")
				w.WriteHeader(status)
				w.Write(cached)
				return
			}

			// Try to acquire lock (prevents concurrent duplicate processing)
			locked, err := store.Lock(ctx, key)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "idempotency lock failed"})
				return
			}
			if !locked {
				w.Header().Set("Retry-After", "1")
				writeJSON(w, http.StatusConflict, errorResponse{Error: "request is already being processed"})
				return
			}

			// Capture the response
			recorder := &responseRecorder{
				ResponseWriter: w,
				body:           &bytes.Buffer{},
			}

			next.ServeHTTP(recorder, r)

			// Store the response for future replays
			if err := store.Store(ctx, key, recorder.body.Bytes(), recorder.statusCode); err != nil {
				// Non-fatal — response already sent to client
				store.Unlock(ctx, key)
			}
		})
	}
}

// responseRecorder captures the response body and status code.
type responseRecorder struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
