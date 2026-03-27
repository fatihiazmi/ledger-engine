package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OutboxEntry represents a pending event in the outbox table.
type OutboxEntry struct {
	ID            int64
	AggregateID   string
	AggregateType string
	EventType     string
	Payload       []byte
	CreatedAt     time.Time
}

// OutboxWriter writes events to both the event store AND the outbox table
// in a single database transaction. This solves the dual-write problem.
type OutboxWriter struct {
	pool *pgxpool.Pool
}

func NewOutboxWriter(pool *pgxpool.Pool) *OutboxWriter {
	return &OutboxWriter{pool: pool}
}

// AppendWithOutbox writes events to the event store AND outbox in one transaction.
// If either fails, both are rolled back — guaranteed consistency.
func (w *OutboxWriter) AppendWithOutbox(ctx context.Context, aggregateID, aggregateType string, expectedVersion int64, events []ledger.StoredEvent) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for i, event := range events {
		version := expectedVersion + int64(i) + 1

		// Write to event store
		_, err := tx.Exec(ctx,
			`INSERT INTO events (aggregate_id, aggregate_type, event_type, version, payload, occurred_at)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			aggregateID, aggregateType, string(event.EventType), version, event.Payload, event.OccurredAt,
		)
		if err != nil {
			return ErrVersionConflict
		}

		// Write to outbox (same transaction!)
		_, err = tx.Exec(ctx,
			`INSERT INTO outbox (aggregate_id, aggregate_type, event_type, payload)
			 VALUES ($1, $2, $3, $4)`,
			aggregateID, aggregateType, string(event.EventType), event.Payload,
		)
		if err != nil {
			return fmt.Errorf("write outbox: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// FetchUnpublished returns the next batch of unpublished outbox entries.
func (w *OutboxWriter) FetchUnpublished(ctx context.Context, batchSize int) ([]OutboxEntry, error) {
	rows, err := w.pool.Query(ctx,
		`SELECT id, aggregate_id, aggregate_type, event_type, payload, created_at
		 FROM outbox
		 WHERE published = FALSE
		 ORDER BY created_at ASC
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`,
		batchSize,
	)
	if err != nil {
		return nil, fmt.Errorf("fetch outbox: %w", err)
	}
	defer rows.Close()

	var entries []OutboxEntry
	for rows.Next() {
		var e OutboxEntry
		if err := rows.Scan(&e.ID, &e.AggregateID, &e.AggregateType, &e.EventType, &e.Payload, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan outbox: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// MarkPublished marks outbox entries as published after successful broker delivery.
func (w *OutboxWriter) MarkPublished(ctx context.Context, ids []int64) error {
	_, err := w.pool.Exec(ctx,
		`UPDATE outbox SET published = TRUE, published_at = NOW() WHERE id = ANY($1)`,
		ids,
	)
	return err
}
