//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatihiazmi/ledger-engine/internal/domain/ledger"
	pgstore "github.com/fatihiazmi/ledger-engine/internal/infra/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func setupPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	migrationsPath, err := filepath.Abs("../../migrations")
	require.NoError(t, err)

	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("ledger_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.WithInitScripts(filepath.Join(migrationsPath, "000001_create_event_store.up.sql")),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, pgContainer.Terminate(ctx))
	})

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)

	t.Cleanup(func() { pool.Close() })

	return pool
}

func TestEventStore_AppendAndLoad(t *testing.T) {
	pool := setupPostgres(t)
	store := pgstore.NewEventStore(pool)
	ctx := context.Background()

	aggregateID := "acc-123"
	payload := mustJSON(t, map[string]any{
		"account_id": aggregateID,
		"name":       "Test Account",
		"currency":   "USD",
	})

	event := ledger.StoredEvent{
		AggregateID:   aggregateID,
		AggregateType: "account",
		EventType:     ledger.EventAccountOpened,
		Payload:       payload,
		OccurredAt:    time.Now().UTC(),
	}

	t.Run("appends and loads events", func(t *testing.T) {
		err := store.Append(ctx, aggregateID, "account", 0, []ledger.StoredEvent{event})
		require.NoError(t, err)

		events, err := store.LoadEvents(ctx, aggregateID)
		require.NoError(t, err)
		assert.Len(t, events, 1)
		assert.Equal(t, string(ledger.EventAccountOpened), string(events[0].EventType))
		assert.Equal(t, int64(1), events[0].Version)
	})

	t.Run("appends multiple events with sequential versions", func(t *testing.T) {
		aggID := "acc-456"
		events := []ledger.StoredEvent{
			{EventType: ledger.EventAccountOpened, Payload: payload, OccurredAt: time.Now().UTC()},
			{EventType: ledger.EventTransactionRecorded, Payload: payload, OccurredAt: time.Now().UTC()},
		}

		err := store.Append(ctx, aggID, "account", 0, events)
		require.NoError(t, err)

		loaded, err := store.LoadEvents(ctx, aggID)
		require.NoError(t, err)
		assert.Len(t, loaded, 2)
		assert.Equal(t, int64(1), loaded[0].Version)
		assert.Equal(t, int64(2), loaded[1].Version)
	})

	t.Run("rejects duplicate version (optimistic concurrency)", func(t *testing.T) {
		aggID := "acc-conflict"
		event := ledger.StoredEvent{
			EventType:  ledger.EventAccountOpened,
			Payload:    payload,
			OccurredAt: time.Now().UTC(),
		}

		err := store.Append(ctx, aggID, "account", 0, []ledger.StoredEvent{event})
		require.NoError(t, err)

		// Try to append with same expected version — should conflict
		err = store.Append(ctx, aggID, "account", 0, []ledger.StoredEvent{event})
		assert.ErrorIs(t, err, pgstore.ErrVersionConflict)
	})

	t.Run("loads events from specific version", func(t *testing.T) {
		aggID := "acc-partial"
		for i := 0; i < 5; i++ {
			e := ledger.StoredEvent{
				EventType:  ledger.EventTransactionRecorded,
				Payload:    mustJSON(t, map[string]any{"seq": i}),
				OccurredAt: time.Now().UTC(),
			}
			err := store.Append(ctx, aggID, "account", int64(i), []ledger.StoredEvent{e})
			require.NoError(t, err)
		}

		// Load from version 3 — should get events 4 and 5
		events, err := store.LoadEventsFrom(ctx, aggID, 3)
		require.NoError(t, err)
		assert.Len(t, events, 2)
		assert.Equal(t, int64(4), events[0].Version)
		assert.Equal(t, int64(5), events[1].Version)
	})
}

func TestSnapshotStore(t *testing.T) {
	pool := setupPostgres(t)
	store := pgstore.NewSnapshotStore(pool)
	ctx := context.Background()

	t.Run("saves and loads snapshot", func(t *testing.T) {
		snap := ledger.Snapshot{
			AggregateID: "acc-snap-1",
			Version:     10,
			Payload:     mustJSON(t, map[string]any{"balance": 5000}),
		}

		err := store.Save(ctx, snap)
		require.NoError(t, err)

		loaded, err := store.Load(ctx, "acc-snap-1")
		require.NoError(t, err)
		assert.Equal(t, int64(10), loaded.Version)
	})

	t.Run("upserts snapshot on same aggregate", func(t *testing.T) {
		snap1 := ledger.Snapshot{
			AggregateID: "acc-snap-2",
			Version:     5,
			Payload:     mustJSON(t, map[string]any{"balance": 1000}),
		}
		snap2 := ledger.Snapshot{
			AggregateID: "acc-snap-2",
			Version:     10,
			Payload:     mustJSON(t, map[string]any{"balance": 5000}),
		}

		require.NoError(t, store.Save(ctx, snap1))
		require.NoError(t, store.Save(ctx, snap2))

		loaded, err := store.Load(ctx, "acc-snap-2")
		require.NoError(t, err)
		assert.Equal(t, int64(10), loaded.Version)
	})

	t.Run("returns empty snapshot for unknown aggregate", func(t *testing.T) {
		snap, err := store.Load(ctx, "nonexistent")
		require.NoError(t, err)
		assert.Empty(t, snap.AggregateID)
	})
}

func TestEventReplay_RebuildsBalance(t *testing.T) {
	pool := setupPostgres(t)
	eventStore := pgstore.NewEventStore(pool)
	ctx := context.Background()

	accountID := "acc-replay"

	// Record 100 transactions as events
	expectedBalance := int64(0)
	for i := 0; i < 100; i++ {
		amount := int64((i + 1) * 100) // 100, 200, 300...
		payload := mustJSON(t, map[string]any{
			"entries": []map[string]any{
				{"account_id": accountID, "amount": amount, "type": "CREDIT"},
			},
		})

		event := ledger.StoredEvent{
			EventType:  ledger.EventTransactionRecorded,
			Payload:    payload,
			OccurredAt: time.Now().UTC(),
		}

		err := eventStore.Append(ctx, accountID, "account", int64(i), []ledger.StoredEvent{event})
		require.NoError(t, err)

		expectedBalance += amount
	}

	// Replay all events to derive balance
	events, err := eventStore.LoadEvents(ctx, accountID)
	require.NoError(t, err)
	assert.Len(t, events, 100)

	var replayedBalance int64
	for _, e := range events {
		var payload map[string]any
		require.NoError(t, json.Unmarshal(e.Payload, &payload))

		entries := payload["entries"].([]any)
		for _, entry := range entries {
			entryMap := entry.(map[string]any)
			amount := int64(entryMap["amount"].(float64))
			if entryMap["type"] == "CREDIT" {
				replayedBalance += amount
			} else {
				replayedBalance -= amount
			}
		}
	}

	assert.Equal(t, expectedBalance, replayedBalance,
		fmt.Sprintf("replayed balance should match: expected %d, got %d", expectedBalance, replayedBalance))
}

func TestSnapshot_ReplayFromCheckpoint(t *testing.T) {
	pool := setupPostgres(t)
	eventStore := pgstore.NewEventStore(pool)
	snapStore := pgstore.NewSnapshotStore(pool)
	ctx := context.Background()

	accountID := "acc-snap-replay"

	// Record 100 events
	var balanceAt50 int64
	for i := 0; i < 100; i++ {
		amount := int64(100)
		payload := mustJSON(t, map[string]any{"amount": amount, "type": "CREDIT"})

		event := ledger.StoredEvent{
			EventType:  ledger.EventTransactionRecorded,
			Payload:    payload,
			OccurredAt: time.Now().UTC(),
		}

		err := eventStore.Append(ctx, accountID, "account", int64(i), []ledger.StoredEvent{event})
		require.NoError(t, err)

		if i == 49 {
			balanceAt50 = int64((i + 1) * 100)
		}
	}

	// Save snapshot at version 50
	snapPayload := mustJSON(t, map[string]any{"balance": balanceAt50})
	require.NoError(t, snapStore.Save(ctx, ledger.Snapshot{
		AggregateID: accountID,
		Version:     50,
		Payload:     snapPayload,
	}))

	// Load snapshot + replay remaining events
	snap, err := snapStore.Load(ctx, accountID)
	require.NoError(t, err)
	assert.Equal(t, int64(50), snap.Version)

	var snapData map[string]any
	require.NoError(t, json.Unmarshal(snap.Payload, &snapData))
	balance := int64(snapData["balance"].(float64))

	// Replay events from version 50 onwards
	events, err := eventStore.LoadEventsFrom(ctx, accountID, snap.Version)
	require.NoError(t, err)
	assert.Len(t, events, 50) // events 51-100

	for _, e := range events {
		var p map[string]any
		require.NoError(t, json.Unmarshal(e.Payload, &p))
		balance += int64(p["amount"].(float64))
	}

	// Full replay should equal snapshot + partial replay
	fullEvents, _ := eventStore.LoadEvents(ctx, accountID)
	var fullBalance int64
	for _, e := range fullEvents {
		var p map[string]any
		json.Unmarshal(e.Payload, &p)
		fullBalance += int64(p["amount"].(float64))
	}

	assert.Equal(t, fullBalance, balance,
		"snapshot + partial replay must equal full replay")
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
