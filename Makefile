.PHONY: indexer dbgen build clean migrate-up migrate-down migrate-status test

# Run the event indexer
# Usage:
#   make indexer CONFIG=configs/config.example.yaml
indexer:
	go run ./cmd/indexer -config=$(or $(CONFIG),configs/config.example.yaml)

# Generate GORM models and queries
# Usage:
#   make dbgen CONFIG=configs/config.example.yaml
#   make dbgen CONFIG=configs/config.example.yaml TABLES=scan_cursor,pending_requests
#   make dbgen CONFIG=configs/config.example.yaml STRIP_PREFIX=t_
dbgen:
	go run ./cmd/dbgen \
		-config=$(or $(CONFIG),configs/config.example.yaml) \
		-out=$(or $(OUT),internal/db/postgres/query) \
		-model-pkg=$(or $(MODEL_PKG),model) \
		$(if $(TABLES),-tables=$(TABLES)) \
		$(if $(STRIP_PREFIX),-strip-prefix=$(STRIP_PREFIX))

# Database migrations (requires goose: go install github.com/pressly/goose/v3/cmd/goose@latest)
# Usage:
#   make migrate-up   DB_DSN="postgres://user:pass@localhost:5432/indexer?sslmode=disable"
#   make migrate-down DB_DSN="postgres://user:pass@localhost:5432/indexer?sslmode=disable"
migrate-up:
	goose -dir migrations postgres "$(DB_DSN)" up

migrate-down:
	goose -dir migrations postgres "$(DB_DSN)" down

migrate-status:
	goose -dir migrations postgres "$(DB_DSN)" status

# Run tests
test:
	go test ./...

# Build binaries
build:
	go build -o bin/indexer ./cmd/indexer
	go build -o bin/dbgen ./cmd/dbgen

clean:
	rm -rf bin/
