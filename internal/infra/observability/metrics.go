package observability

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// Metrics holds all application metrics instruments.
type Metrics struct {
	// SLI: Request latency (histogram)
	RequestDuration otelmetric.Float64Histogram
	// SLI: Request count by status
	RequestCount otelmetric.Int64Counter
	// SLI: Active accounts gauge
	ActiveAccounts otelmetric.Int64UpDownCounter
	// Business: Transaction amount
	TransactionAmount otelmetric.Int64Counter
	// Business: Transfer count by state (completed, failed, compensated)
	TransfersByState otelmetric.Int64Counter
	// Outbox: pending events
	OutboxPending otelmetric.Int64UpDownCounter
	// Outbox: published events
	OutboxPublished otelmetric.Int64Counter
}

// SetupMetrics initializes OpenTelemetry metrics with Prometheus exporter.
// Returns the Prometheus exporter for the HTTP handler.
func SetupMetrics() (*Metrics, *prometheus.Exporter, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, nil, err
	}

	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	otel.SetMeterProvider(provider)

	meter := provider.Meter("ledger-engine")

	requestDuration, _ := meter.Float64Histogram("http_request_duration_seconds",
		otelmetric.WithDescription("HTTP request latency in seconds"),
		otelmetric.WithUnit("s"),
	)

	requestCount, _ := meter.Int64Counter("http_requests_total",
		otelmetric.WithDescription("Total HTTP requests"),
	)

	activeAccounts, _ := meter.Int64UpDownCounter("ledger_active_accounts",
		otelmetric.WithDescription("Number of active accounts"),
	)

	transactionAmount, _ := meter.Int64Counter("ledger_transaction_amount_cents_total",
		otelmetric.WithDescription("Total transaction amount in cents"),
	)

	transfersByState, _ := meter.Int64Counter("ledger_transfers_total",
		otelmetric.WithDescription("Total transfers by final state"),
	)

	outboxPending, _ := meter.Int64UpDownCounter("outbox_pending_events",
		otelmetric.WithDescription("Pending outbox events"),
	)

	outboxPublished, _ := meter.Int64Counter("outbox_published_total",
		otelmetric.WithDescription("Total outbox events published"),
	)

	return &Metrics{
		RequestDuration:   requestDuration,
		RequestCount:      requestCount,
		ActiveAccounts:    activeAccounts,
		TransactionAmount: transactionAmount,
		TransfersByState:  transfersByState,
		OutboxPending:     outboxPending,
		OutboxPublished:   outboxPublished,
	}, exporter, nil
}

// RecordRequest records HTTP request metrics.
func (m *Metrics) RecordRequest(ctx context.Context, method, path string, status int, duration time.Duration) {
	attrs := otelmetric.WithAttributes(
		attribute.String("method", method),
		attribute.String("path", path),
		attribute.Int("status", status),
	)
	m.RequestDuration.Record(ctx, duration.Seconds(), attrs)
	m.RequestCount.Add(ctx, 1, attrs)
}

// RecordTransfer records a transfer outcome.
func (m *Metrics) RecordTransfer(ctx context.Context, state string) {
	m.TransfersByState.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.String("state", state),
	))
}
