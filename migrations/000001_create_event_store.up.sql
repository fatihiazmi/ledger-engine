-- Event store: append-only log of all domain events (source of truth)
CREATE TABLE IF NOT EXISTS events (
    id              BIGSERIAL PRIMARY KEY,
    aggregate_id    TEXT NOT NULL,
    aggregate_type  TEXT NOT NULL,
    event_type      TEXT NOT NULL,
    version         BIGINT NOT NULL,
    payload         JSONB NOT NULL,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Optimistic concurrency: unique version per aggregate
    CONSTRAINT uq_aggregate_version UNIQUE (aggregate_id, version)
);

CREATE INDEX idx_events_aggregate_id ON events (aggregate_id, version);
CREATE INDEX idx_events_event_type ON events (event_type);
CREATE INDEX idx_events_occurred_at ON events (occurred_at);

-- Snapshots: periodic state captures for faster replay
CREATE TABLE IF NOT EXISTS snapshots (
    aggregate_id    TEXT PRIMARY KEY,
    version         BIGINT NOT NULL,
    payload         JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
