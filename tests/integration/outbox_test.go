//go:build integration

package integration

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fatihiazmi/ledger-engine/internal/app"
	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	pgstore "github.com/fatihiazmi/ledger-engine/internal/infra/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// recordingPublisher captures published events for test assertions.
type recordingPublisher struct {
	mu     sync.Mutex
	events []publishedEvent
}

type publishedEvent struct {
	EventType   string
	AggregateID string
	Payload     []byte
}

func (p *recordingPublisher) Publish(_ context.Context, eventType, aggregateID string, payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, publishedEvent{eventType, aggregateID, payload})
	return nil
}

func (p *recordingPublisher) Events() []publishedEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]publishedEvent, len(p.events))
	copy(cp, p.events)
	return cp
}

func setupOutboxTest(t *testing.T) (*pgxpool.Pool, *pgstore.OutboxWriter) {
	t.Helper()
	ctx := context.Background()

	migrationsPath, err := filepath.Abs("../../migrations")
	require.NoError(t, err)

	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("ledger_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithInitScripts(
			filepath.Join(migrationsPath, "000001_create_event_store.up.sql"),
			filepath.Join(migrationsPath, "000002_create_read_models.up.sql"),
			filepath.Join(migrationsPath, "000003_create_accounts_write.up.sql"),
			filepath.Join(migrationsPath, "000004_create_outbox.up.sql"),
		),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { pgContainer.Terminate(ctx) })

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	return pool, pgstore.NewOutboxWriter(pool)
}

func TestOutbox_AtomicWriteWithEventStore(t *testing.T) {
	pool, outbox := setupOutboxTest(t)
	ctx := context.Background()

	t.Run("writes to both event store and outbox atomically", func(t *testing.T) {
		event := ledger.StoredEvent{
			EventType:  ledger.EventAccountOpened,
			Payload:    mustJSON(t, map[string]any{"name": "Test"}),
			OccurredAt: time.Now().UTC(),
		}

		err := outbox.AppendWithOutbox(ctx, "acc-1", "account", 0, []ledger.StoredEvent{event})
		require.NoError(t, err)

		// Verify event in event store
		var eventCount int
		pool.QueryRow(ctx, "SELECT COUNT(*) FROM events WHERE aggregate_id = 'acc-1'").Scan(&eventCount)
		assert.Equal(t, 1, eventCount)

		// Verify event in outbox
		var outboxCount int
		pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox WHERE aggregate_id = 'acc-1'").Scan(&outboxCount)
		assert.Equal(t, 1, outboxCount)

		// Outbox entry should be unpublished
		var published bool
		pool.QueryRow(ctx, "SELECT published FROM outbox WHERE aggregate_id = 'acc-1'").Scan(&published)
		assert.False(t, published)
	})
}

func TestOutbox_WorkerPublishesEvents(t *testing.T) {
	_, outbox := setupOutboxTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Write 3 events to outbox
	for i := 0; i < 3; i++ {
		event := ledger.StoredEvent{
			EventType:  ledger.EventTransactionRecorded,
			Payload:    mustJSON(t, map[string]any{"seq": i}),
			OccurredAt: time.Now().UTC(),
		}
		err := outbox.AppendWithOutbox(ctx, "acc-worker", "account", int64(i), []ledger.StoredEvent{event})
		require.NoError(t, err)
	}

	// Start worker with recording publisher
	pub := &recordingPublisher{}
	worker := app.NewOutboxWorker(outbox, pub)
	go worker.Start(ctx)

	// Wait for worker to process
	time.Sleep(3 * time.Second)
	cancel()

	// All 3 events should be published
	events := pub.Events()
	assert.Len(t, events, 3, "worker should have published all 3 events")

	// All should be marked as published in DB
	unpublished, _ := outbox.FetchUnpublished(context.Background(), 100)
	assert.Len(t, unpublished, 0, "no unpublished entries should remain")
}

func TestOutbox_IdempotentConsumer(t *testing.T) {
	_, outbox := setupOutboxTest(t)
	ctx := context.Background()

	// Write same event
	event := ledger.StoredEvent{
		EventType:  ledger.EventAccountOpened,
		Payload:    mustJSON(t, map[string]any{"name": "Idempotent"}),
		OccurredAt: time.Now().UTC(),
	}
	require.NoError(t, outbox.AppendWithOutbox(ctx, "acc-idem", "account", 0, []ledger.StoredEvent{event}))

	// Fetch, publish, but DON'T mark as published (simulating crash)
	entries, err := outbox.FetchUnpublished(ctx, 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	// Fetch again — should get the same entry (at-least-once)
	entries2, err := outbox.FetchUnpublished(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, entries2, 1, "unpublished entry should be re-fetched for retry")

	// Mark as published
	require.NoError(t, outbox.MarkPublished(ctx, []int64{entries[0].ID}))

	// Now fetch should return nothing
	entries3, err := outbox.FetchUnpublished(ctx, 10)
	require.NoError(t, err)
	assert.Len(t, entries3, 0, "published entry should not be re-fetched")
}
