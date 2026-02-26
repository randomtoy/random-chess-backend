DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/random_chess?sslmode=disable

.PHONY: build migrate-up migrate-down migrate-status test test-integration

build:
	CGO_ENABLED=0 go build -o bin/api ./cmd/api
	CGO_ENABLED=0 go build -o bin/migrate ./cmd/migrate

migrate-up:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/migrate up

migrate-down:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/migrate down

migrate-status:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/migrate status

test:
	go test -race ./...

# test-integration requires Docker (spins up postgres via testcontainers).
test-integration:
	go test -race -tags integration -timeout 120s ./...
