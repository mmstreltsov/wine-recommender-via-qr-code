# CLI запускается как отдельный tool module, не добавляя драйверы в runtime go.mod.
GOOSE := go run -tags=no_clickhouse,no_mssql,no_mysql,no_sqlite3,no_libsql,no_vertica,no_ydb github.com/pressly/goose/v3/cmd/goose@v3.29.0
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
	@: "$${DATABASE_URL:?DATABASE_URL is required}"; \
	if ! GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$DATABASE_URL" $(GOOSE) -env none -dir $(MIGRATIONS_DIR) up >/dev/null 2>&1; then \
		printf '%s\n' 'ERROR operation=migrate_up code=internal' >&2; exit 1; \
	fi

migrate-down:
	@: "$${DATABASE_URL:?DATABASE_URL is required}"; \
	if ! GOOSE_DRIVER=postgres GOOSE_DBSTRING="$$DATABASE_URL" $(GOOSE) -env none -dir $(MIGRATIONS_DIR) down >/dev/null 2>&1; then \
		printf '%s\n' 'ERROR operation=migrate_down code=internal' >&2; exit 1; \
	fi

run:
	go run ./cmd/server
