# Дефолтный DSN — ТОЛЬКО локальная dev-БД из docker-compose.yml (dev-значения).
# Prod: DATABASE_URL передаётся через env и дефолт не используется.
DATABASE_URL ?= postgres://svoyshef:svoyshef@localhost:5432/svoyshef
GOOSE := go run github.com/pressly/goose/v3/cmd/goose
MIGRATIONS_DIR := migrations

.PHONY: build test lint vet migrate-up migrate-down run

build:
	go build ./...

test:
	go test ./...

lint:
	golangci-lint run

vet:
	go vet ./...

migrate-up:
	$(GOOSE) -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" up

migrate-down:
	$(GOOSE) -dir $(MIGRATIONS_DIR) postgres "$(DATABASE_URL)" down

run:
	go run ./cmd/server
