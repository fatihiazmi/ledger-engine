package app

import (
	"context"
	"log"
	"time"

	pgstore "github.com/fatihiazmi/ledger-engine/internal/infra/postgres"
)

// EventPublisher is the port for publishing events to an external broker.
// Implementations: Kafka, NATS, in-memory (for testing), log (for dev).
type EventPublisher interface {
	Publish(ctx context.Context, eventType, aggregateID string, payload []byte) error
}

// OutboxWorker polls the outbox table and publishes events to the broker.
// Runs as a background goroutine. At-least-once delivery guarantee.
type OutboxWorker struct {
	outbox    *pgstore.OutboxWriter
	publisher EventPublisher
	interval  time.Duration
	batchSize int
}

func NewOutboxWorker(outbox *pgstore.OutboxWriter, publisher EventPublisher) *OutboxWorker {
	return &OutboxWorker{
		outbox:    outbox,
		publisher: publisher,
		interval:  1 * time.Second,
		batchSize: 50,
	}
}

// Start begins polling the outbox. Blocks until context is cancelled.
func (w *OutboxWorker) Start(ctx context.Context) {
	log.Println("Outbox worker started")
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Outbox worker stopped")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *OutboxWorker) processBatch(ctx context.Context) {
	entries, err := w.outbox.FetchUnpublished(ctx, w.batchSize)
	if err != nil {
		log.Printf("outbox: fetch error: %v", err)
		return
	}

	if len(entries) == 0 {
		return
	}

	var publishedIDs []int64
	for _, entry := range entries {
		if err := w.publisher.Publish(ctx, entry.EventType, entry.AggregateID, entry.Payload); err != nil {
			log.Printf("outbox: publish error for entry %d: %v", entry.ID, err)
			continue // skip this one, retry next poll
		}
		publishedIDs = append(publishedIDs, entry.ID)
	}

	if len(publishedIDs) > 0 {
		if err := w.outbox.MarkPublished(ctx, publishedIDs); err != nil {
			log.Printf("outbox: mark published error: %v", err)
		} else {
			log.Printf("outbox: published %d events", len(publishedIDs))
		}
	}
}
