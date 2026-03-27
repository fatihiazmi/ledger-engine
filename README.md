# Ledger Engine

A double-entry payment ledger system built with enterprise-grade patterns. Designed as a learning project to master production fintech architecture.

## Architecture

```
cmd/api/           Entry point — REST API + embedded frontend
internal/
├── domain/
│   ├── ledger/    Core: Money, Transaction, Entry (double-entry invariant)
│   └── account/   Account entity with lifecycle states
├── app/           Application services (use cases, command handlers)
├── infra/
│   ├── postgres/  Event store, snapshot store, account repository
│   ├── http/      REST handlers, router
│   └── inmemory/  In-memory fakes for unit testing
└── projection/    CQRS read models, projector, query service
web/static/        Frontend dashboard
migrations/        Postgres schema migrations
tests/
├── property/      Property-based tests
└── integration/   Integration tests (testcontainers)
```

## Patterns & Paradigms

| Pattern | Where | What it does |
|---------|-------|-------------|
| **Domain-Driven Design** | `internal/domain/` | Bounded contexts, aggregates, value objects, domain events |
| **Domain Primitives** | `Money`, `AccountID`, etc. | Immutable, validated at creation — if it exists, it's valid |
| **Double-Entry Bookkeeping** | `Transaction` | Every transaction has balanced debit/credit entries |
| **Hexagonal Architecture** | Repository interfaces | Domain defines ports, infrastructure provides adapters |
| **Event Sourcing** | `EventStore` | Append-only event log as source of truth |
| **CQRS** | Write model + Read projections | Separate optimized models for commands and queries |
| **Property-Based Testing** | `*_test.go` | Invariants verified with random inputs (rapid) |
| **Secure by Design** | Throughout | Constructor validation, immutability, fail-fast contracts |

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.23+ |
| Database | PostgreSQL 16 |
| Cache | Redis 7 |
| PBT | pgregory.net/rapid |
| HTTP | chi router |
| Testing | testify + testcontainers-go |
| Frontend | Vanilla HTML/CSS/JS (embedded) |

## Quick Start

```bash
# Start infrastructure
make up

# Run migrations
docker compose exec postgres psql -U ledger -d ledger_db \
  -f /dev/stdin < migrations/000001_create_event_store.up.sql
docker compose exec postgres psql -U ledger -d ledger_db \
  -f /dev/stdin < migrations/000002_create_read_models.up.sql
docker compose exec postgres psql -U ledger -d ledger_db \
  -f /dev/stdin < migrations/000003_create_accounts_write.up.sql

# Run the server
make run-api

# Open dashboard
open http://localhost:8090
```

## API Endpoints

```
GET    /api/v1/accounts                    List all accounts
POST   /api/v1/accounts                    Create account
GET    /api/v1/accounts/{id}               Get account balance
GET    /api/v1/accounts/{id}/transactions  Transaction history
POST   /api/v1/transactions               Record transaction
POST   /api/v1/deposit                     Add funds to account
```

## Testing

```bash
make test          # All unit tests (57 tests, 11 PBT)
make test-unit     # Domain tests only
make test-race     # Race condition detection
make test-int      # Integration tests (requires Docker)
make test-cover    # Coverage report
```

## Key Invariants (verified by PBT)

- Credits always equal debits in every transaction
- Money is never created or destroyed during transfers
- Account balance equals sum of all entries
- Unbalanced transactions are always rejected
- Debit exceeding balance is always rejected (asset accounts)
- Event replay produces same state as direct computation

## Project Phases

- [x] Phase 1: Domain Foundation + Property-Based Testing
- [x] Phase 2: Event Sourcing (Postgres event store, snapshots)
- [x] Phase 3: CQRS (Command/Query separation, read projections)
- [x] Phase 4: REST API + Frontend Dashboard
- [x] Phase 5: Idempotency Keys
- [x] Phase 6: Saga Pattern (Transfers with compensation)
- [x] Phase 7: Outbox Pattern (Reliable event publishing)
- [x] Phase 8: Observability (OpenTelemetry, SLOs)

## License

MIT
