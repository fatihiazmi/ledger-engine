.PHONY: up down down-clean restart rebuild logs ps \
       migrate-up migrate-down db-shell db-reset \
       run-api run-worker build \
       test test-unit test-pbt test-int test-cover test-race \
       lint fmt vet check dev fresh

# === Infrastructure ===
up:
	docker compose up -d

down:
	docker compose down

down-clean:
	docker compose down -v

restart:
	docker compose restart

rebuild:
	docker compose up -d --build --force-recreate

logs:
	docker compose logs -f

ps:
	docker compose ps

# === Database ===
migrate-up:
	go run cmd/migrate/main.go up

migrate-down:
	go run cmd/migrate/main.go down 1

migrate-create:
	migrate create -ext sql -dir migrations -seq $(name)

db-shell:
	docker compose exec postgres psql -U ledger -d ledger_db

db-reset: down-clean up
	@sleep 2
	@$(MAKE) migrate-up

# === Application ===
run-api:
	go run cmd/api/main.go

run-worker:
	go run cmd/worker/main.go

build:
	go build -o bin/api cmd/api/main.go
	go build -o bin/worker cmd/worker/main.go

# === Testing ===
test:
	go test ./...

test-unit:
	go test ./internal/domain/...

test-pbt:
	go test ./tests/property/...

test-int:
	go test -tags=integration ./tests/integration/...

test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

test-race:
	go test -race ./...

# === Code Quality ===
lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

check: fmt vet test

# === Shortcuts ===
dev: up
	@sleep 2
	@$(MAKE) migrate-up
	@$(MAKE) run-api

fresh: db-reset run-api
