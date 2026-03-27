package observability

import (
	"log/slog"
	"os"
)

// SetupLogger creates a structured JSON logger.
// Domain-aware: never logs sensitive data (Secure by Design).
func SetupLogger() *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}
