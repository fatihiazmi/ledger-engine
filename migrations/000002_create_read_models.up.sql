-- CQRS Read Models: denormalized views optimized for queries
-- These are projections derived from the event store (source of truth)

CREATE TABLE IF NOT EXISTS account_balances (
    account_id    TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    account_type  TEXT NOT NULL,
    currency      TEXT NOT NULL,
    balance       BIGINT NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'ACTIVE',
    version       BIGINT NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS transaction_history (
    id              BIGSERIAL PRIMARY KEY,
    transaction_id  TEXT NOT NULL,
    account_id      TEXT NOT NULL,
    entry_type      TEXT NOT NULL,
    amount          BIGINT NOT NULL,
    currency        TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    balance_after   BIGINT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_txn_history_account ON transaction_history (account_id, created_at DESC);
CREATE INDEX idx_txn_history_txn_id ON transaction_history (transaction_id);
