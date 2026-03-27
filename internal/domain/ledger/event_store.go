package ledger

import (
	"context"
	"time"
)

// StoredEvent is a persisted event with metadata for the event store.
type StoredEvent struct {
	ID            string
	AggregateID   string
	AggregateType string
	EventType     EventType
	Version       int64
	Payload       []byte    // JSON-encoded domain event
	OccurredAt    time.Time
}

// EventStore defines the port for append-only event persistence.
// Source of truth in an event-sourced system.
type EventStore interface {
	// Append persists events for an aggregate. Returns error if version conflict (optimistic concurrency).
	Append(ctx context.Context, aggregateID string, aggregateType string, expectedVersion int64, events []StoredEvent) error

	// LoadEvents retrieves all events for an aggregate, ordered by version.
	LoadEvents(ctx context.Context, aggregateID string) ([]StoredEvent, error)

	// LoadEventsFrom retrieves events starting from a specific version (for snapshot-based replay).
	LoadEventsFrom(ctx context.Context, aggregateID string, fromVersion int64) ([]StoredEvent, error)
}

// Snapshot represents a point-in-time state capture for faster replay.
type Snapshot struct {
	AggregateID string
	Version     int64
	Payload     []byte // JSON-encoded aggregate state
}

// SnapshotStore defines the port for snapshot persistence.
type SnapshotStore interface {
	Save(ctx context.Context, snapshot Snapshot) error
	Load(ctx context.Context, aggregateID string) (Snapshot, error)
}
