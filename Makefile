.PHONY: indexer dbgen build clean test test-integration
.PHONY: migrate-up migrate-down migrate-status
.PHONY: migrate-mysql-up migrate-mysql-down migrate-mysql-status
.PHONY: docker-build docker-up-postgres docker-up-mysql docker-down docker-logs

# Run the event indexer
# Usage:
#   make indexer CONFIG=configs/config.yaml
indexer:
	go run ./cmd/indexer -config=$(or $(CONFIG),configs/config.yaml)

# Generate GORM models and queries
# Usage:
#   make dbgen CONFIG=configs/config.yaml
#   make dbgen CONFIG=configs/config.yaml TABLES=scan_cursor,pending_requests
#   make dbgen CONFIG=configs/config.yaml STRIP_PREFIX=t_
dbgen:
	go run ./cmd/dbgen \
		-config=$(or $(CONFIG),configs/config.yaml) \
		-out=$(or $(OUT),internal/db/query) \
		-model-pkg=$(or $(MODEL_PKG),model) \
		$(if $(TABLES),-tables=$(TABLES)) \
		$(if $(STRIP_PREFIX),-strip-prefix=$(STRIP_PREFIX))

# PostgreSQL migrations (requires goose: go install github.com/pressly/goose/v3/cmd/goose@latest)
# Usage:
#   make migrate-up   DB_DSN="postgres://user:pass@localhost:5432/indexer?sslmode=disable"
#   make migrate-down DB_DSN="postgres://user:pass@localhost:5432/indexer?sslmode=disable"
migrate-up:
	goose -dir migrations/postgres postgres "$(DB_DSN)" up

migrate-down:
	goose -dir migrations/postgres postgres "$(DB_DSN)" down

migrate-status:
	goose -dir migrations/postgres postgres "$(DB_DSN)" status

# MySQL migrations
# Usage:
#   make migrate-mysql-up   DB_DSN="user:pass@tcp(localhost:3306)/indexer?parseTime=true"
#   make migrate-mysql-down DB_DSN="user:pass@tcp(localhost:3306)/indexer?parseTime=true"
migrate-mysql-up:
	goose -dir migrations/mysql mysql "$(DB_DSN)" up

migrate-mysql-down:
	goose -dir migrations/mysql mysql "$(DB_DSN)" down

migrate-mysql-status:
	goose -dir migrations/mysql mysql "$(DB_DSN)" status

# Run tests
test:
	go test ./...

# Run integration tests (requires Docker)
test-integration:
	cd tests/integration && go test -v -count=1 ./...

# Build binaries
build:
	go build -o bin/indexer ./cmd/indexer
	go build -o bin/dbgen ./cmd/dbgen

clean:
	rm -rf bin/

# Docker
# Usage:
#   make docker-build                 # build image tagged evm-indexer:latest
#   make docker-up-postgres           # start postgres + indexer
#   make docker-up-mysql              # start mysql + indexer
#   make docker-down                  # stop and remove both profiles
#   make docker-logs                  # tail indexer logs
docker-build:
	docker compose build indexer

docker-up-postgres:
	docker compose --profile postgres up -d

docker-up-mysql:
	docker compose --profile mysql up -d

docker-down:
	docker compose --profile postgres --profile mysql down

docker-logs:
	docker compose logs -f indexer
