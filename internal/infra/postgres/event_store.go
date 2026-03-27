package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrVersionConflict = errors.New("event version conflict: aggregate was modified concurrently")

// EventStore implements ledger.EventStore using Postgres.
// Append-only, optimistic concurrency via unique version constraint.
type EventStore struct {
	pool *pgxpool.Pool
}

func NewEventStore(pool *pgxpool.Pool) *EventStore {
	return &EventStore{pool: pool}
}

func (s *EventStore) Append(ctx context.Context, aggregateID string, aggregateType string, expectedVersion int64, events []ledger.StoredEvent) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for i, event := range events {
		version := expectedVersion + int64(i) + 1
		_, err := tx.Exec(ctx,
			`INSERT INTO events (aggregate_id, aggregate_type, event_type, version, payload, occurred_at)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			aggregateID, aggregateType, string(event.EventType), version, event.Payload, event.OccurredAt,
		)
		if err != nil {
			return ErrVersionConflict
		}
	}

	return tx.Commit(ctx)
}

func (s *EventStore) LoadEvents(ctx context.Context, aggregateID string) ([]ledger.StoredEvent, error) {
	return s.LoadEventsFrom(ctx, aggregateID, 0)
}

func (s *EventStore) LoadEventsFrom(ctx context.Context, aggregateID string, fromVersion int64) ([]ledger.StoredEvent, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, aggregate_id, aggregate_type, event_type, version, payload, occurred_at
		 FROM events
		 WHERE aggregate_id = $1 AND version > $2
		 ORDER BY version ASC`,
		aggregateID, fromVersion,
	)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []ledger.StoredEvent
	for rows.Next() {
		var (
			e     ledger.StoredEvent
			dbID  int64
		)
		if err := rows.Scan(&dbID, &e.AggregateID, &e.AggregateType, &e.EventType, &e.Version, &e.Payload, &e.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		e.ID = fmt.Sprintf("%d", dbID)
		events = append(events, e)
	}

	return events, rows.Err()
}

// SnapshotStore implements ledger.SnapshotStore using Postgres.
type SnapshotStore struct {
	pool *pgxpool.Pool
}

func NewSnapshotStore(pool *pgxpool.Pool) *SnapshotStore {
	return &SnapshotStore{pool: pool}
}

func (s *SnapshotStore) Save(ctx context.Context, snapshot ledger.Snapshot) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO snapshots (aggregate_id, version, payload)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (aggregate_id) DO UPDATE SET version = $2, payload = $3, created_at = NOW()`,
		snapshot.AggregateID, snapshot.Version, snapshot.Payload,
	)
	return err
}

func (s *SnapshotStore) Load(ctx context.Context, aggregateID string) (ledger.Snapshot, error) {
	var snap ledger.Snapshot
	err := s.pool.QueryRow(ctx,
		`SELECT aggregate_id, version, payload FROM snapshots WHERE aggregate_id = $1`,
		aggregateID,
	).Scan(&snap.AggregateID, &snap.Version, &snap.Payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return ledger.Snapshot{}, nil
	}
	return snap, err
}
