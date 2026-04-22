# evm-indexer

A Go project template for building EVM chain event indexers. Clone this repo, customize your handler, and start monitoring on-chain contract events.

## Quick Start

```bash
# 1. Clone and detach from template
git clone https://github.com/kay404/evm-indexer.git my-oracle
cd my-oracle
rm -rf .git

# 2. Set your module path
#    Replace ALL occurrences of "github.com/kay404/evm-indexer" in go.mod and *.go files
#    with your own module path, e.g. "github.com/yourname/my-oracle"
find . -name "*.go" -o -name "go.mod" | xargs sed -i '' 's|github.com/kay404/evm-indexer|github.com/yourname/my-oracle|g'

# 3. Rename entry point
mv cmd/indexer cmd/my-oracle

# 4. Edit handler
#    internal/handler/handler.go → Fill in Filter() and HandleLogs()

# 5. Copy and edit config
cp configs/config.example.yaml configs/config.yaml
#    → Set rpc_url, chain_id, database connection
#    → Set databases.driver to "postgres" (default) or "mysql"

# 6. Install goose and run migrations
go install github.com/pressly/goose/v3/cmd/goose@latest
# PostgreSQL:
make migrate-up DB_DSN="postgres://user:pass@localhost:5432/mydb?sslmode=disable"
# MySQL:
# make migrate-mysql-up DB_DSN="user:pass@tcp(localhost:3306)/mydb?parseTime=true"

# 7. Resolve dependencies and run
go mod tidy
make indexer
```

## Project Structure

```
├── cmd/
│   ├── indexer/main.go          # Entry point (rename to your project)
│   └── dbgen/main.go            # GORM code generation tool
├── internal/
│   ├── config/                  # Config types, YAML loader, env helpers
│   ├── postgres/                # PostgreSQL connection with Option pattern
│   ├── mysql/                   # MySQL connection with Option pattern
│   ├── indexer/                 # Engine, handler interface, cursor
│   ├── dbgen/                   # GORM Gen wrapper
│   └── handler/                 # Your EventHandler implementation
├── configs/
│   └── config.example.yaml
├── migrations/
│   ├── postgres/                # PostgreSQL migrations
│   └── mysql/                   # MySQL migrations
├── tests/
│   └── integration/             # Integration tests (separate Go module, requires Docker)
├── Dockerfile                   # Multi-stage build for the indexer binary
├── docker-compose.yml           # Compose stack with postgres/mysql profiles
├── .dockerignore
└── Makefile
```

## How It Works

The indexer engine polls the chain at a configurable interval:

1. Get latest block → compute safe block (`latest - delay_block`)
2. Load cursor per handler → scan events in batches via `eth_getLogs`
3. Dispatch matched logs to your `HandleLogs()` implementation
4. Advance cursor after successful processing

Each handler has its own independent cursor. If handler B fails, handler A's progress is unaffected.

## Configure Contracts

Contract addresses and events are defined in config, not in code:

```yaml
contracts:
  - name: "transfer-watcher"
    address: "0xdAC17F958D2ee523a2206206994597C13D831ec7"
    events:
      - "Transfer(address,address,uint256)"

  - name: "approval-watcher"
    address: "0xdAC17F958D2ee523a2206206994597C13D831ec7"
    events:
      - "Approval(address,address,uint256)"
```

Each entry becomes an independent handler with its own cursor. Event signatures are automatically hashed to topic0.

## Add Business Logic

Edit `internal/handler/handler.go` → `HandleLogs()`:

```go
func (h *ContractHandler) HandleLogs(ctx context.Context, logs []types.Log) error {
    for _, lg := range logs {
        // Your business logic here.
        // MUST be idempotent — the engine provides at-least-once delivery.
    }
    return nil
}
```

## Configuration

### YAML

Copy `configs/config.example.yaml` to `configs/config.yaml` and fill in your values.

### Environment Variables

Environment variables override YAML values. Prefix: `INDEXER_`.

**Indexer:**

| Variable | Description |
|----------|-------------|
| `INDEXER_RPC_URL` | Chain RPC endpoint |
| `INDEXER_CHAIN_ID` | Chain ID |
| `INDEXER_POLL_INTERVAL` | Poll interval (e.g. `3s`) |
| `INDEXER_DELAY_BLOCK` | Safe block delay |
| `INDEXER_START_BLOCK` | Block to start scanning from |

**Database driver:**

| Variable | Description |
|----------|-------------|
| `INDEXER_DB_DRIVER` | `postgres` (default) or `mysql` |

**PostgreSQL** (`databases.driver: "postgres"`):

| Variable | Description |
|----------|-------------|
| `INDEXER_POSTGRES_HOST` | Postgres host |
| `INDEXER_POSTGRES_PORT` | Postgres port |
| `INDEXER_POSTGRES_DBNAME` | Database name |
| `INDEXER_POSTGRES_USERNAME` | Database user |
| `INDEXER_POSTGRES_PASSWORD` | Database password |

**MySQL** (`databases.driver: "mysql"`):

| Variable | Description |
|----------|-------------|
| `INDEXER_MYSQL_HOST` | MySQL host |
| `INDEXER_MYSQL_PORT` | MySQL port |
| `INDEXER_MYSQL_DBNAME` | Database name |
| `INDEXER_MYSQL_USERNAME` | Database user |
| `INDEXER_MYSQL_PASSWORD` | Database password |
| `INDEXER_MYSQL_CHARSET` | Charset (default: `utf8mb4`) |
| `INDEXER_MYSQL_LOC` | Timezone (default: `UTC`, e.g. `Asia/Shanghai`) |

## Docker

The repo ships with a multi-stage `Dockerfile` and a `docker-compose.yml` that bundles the indexer together with either PostgreSQL or MySQL via Compose profiles.

```bash
# 1. Build the image
make docker-build

# 2. Make sure configs/config.yaml exists and the database host points at the
#    compose service name (`postgres` or `mysql`), not 127.0.0.1.

# 3. Bring up one stack (choose ONE profile)
make docker-up-postgres
# or
make docker-up-mysql

# 4. Run migrations against the containerised DB
#    PostgreSQL:
make migrate-up    DB_DSN="postgres://postgres:postgres@localhost:5432/indexer?sslmode=disable"
#    MySQL:
make migrate-mysql-up DB_DSN="indexer:indexer@tcp(localhost:3306)/indexer?parseTime=true"

# 5. Tail logs / stop the stack
make docker-logs
make docker-down
```

Notes:

- The image runs as a non-root user and only contains the indexer binary plus `ca-certificates` and `tzdata`.
- `configs/config.yaml` is mounted read-only into the container at `/app/configs/config.yaml` — edit it on the host.
- Database credentials and ports can be overridden with env vars (e.g. `POSTGRES_PASSWORD`, `MYSQL_PORT`). See `docker-compose.yml` for the full list.
- Both profiles can coexist in the same `docker-compose.yml`; Compose only starts the profile you pick.

## Make Targets

| Target | Description |
|--------|-------------|
| `make indexer` | Run the indexer |
| `make build` | Build binaries to `bin/` |
| `make dbgen` | Generate GORM models and queries |
| `make migrate-up` | Run PostgreSQL migrations (requires [goose](https://github.com/pressly/goose)) |
| `make migrate-down` | Rollback last PostgreSQL migration |
| `make migrate-status` | Show PostgreSQL migration status |
| `make migrate-mysql-up` | Run MySQL migrations |
| `make migrate-mysql-down` | Rollback last MySQL migration |
| `make migrate-mysql-status` | Show MySQL migration status |
| `make test` | Run unit tests |
| `make test-integration` | Run integration tests (requires Docker) |
| `make docker-build` | Build the indexer image (`evm-indexer:latest`) |
| `make docker-up-postgres` | Start the indexer + PostgreSQL stack |
| `make docker-up-mysql` | Start the indexer + MySQL stack |
| `make docker-down` | Stop and remove both compose profiles |
| `make docker-logs` | Tail indexer container logs |
| `make clean` | Remove build artifacts |

## Design Decisions

- **At-least-once delivery**: handlers may receive the same logs on retry. Implement idempotency using unique constraints or deduplication by `(txHash, logIndex)`.
- **Reorg strategy**: relies on `delay_block` to avoid short-lived reorgs. No block hash verification. If your business is reorg-sensitive, add your own checks.
- **Per-handler cursors**: each handler tracks its own progress independently via the `scan_cursor` table.
- **Fail fast**: empty filters, missing config fields, and invalid chain IDs are rejected at startup.
- **Database agnostic**: supports PostgreSQL and MySQL via GORM. The cursor store (`GormCursorStore`) works with any GORM-compatible backend. Switch by setting `databases.driver` in config.
- **Config validation**: `postgres.NewDB()` / `mysql.NewDB()` applies defaults then validates; `indexer.New()` validates config and handler filters before connecting to RPC.
