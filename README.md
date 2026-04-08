# evm-indexer

A Go template for building EVM chain event indexers. Clone, customize your handler, and start monitoring on-chain contract events.

## Quick Start

```bash
# 1. Clone
git clone https://github.com/kay404/evm-indexer.git my-oracle
cd my-oracle

# 2. Edit handler
#    internal/handler/handler.go → Fill in Filter() and HandleLogs()

# 3. Edit config
#    configs/config.example.yaml → Set rpc_url, chain_id, postgres

# 4. Run migrations
make migrate-up DB_DSN="postgres://user:pass@localhost:5432/mydb?sslmode=disable"

# 5. Run
make indexer CONFIG=configs/config.example.yaml
```

## Project Structure

```
├── cmd/
│   ├── indexer/main.go          # Entry point
│   └── dbgen/main.go            # GORM code generation tool
├── internal/
│   ├── config/                  # Config types, YAML loader, env helpers
│   ├── postgres/                # DB connection with Option pattern
│   ├── indexer/                 # Engine, handler interface, cursor
│   ├── dbgen/                   # GORM Gen wrapper
│   └── handler/                 # Your EventHandler implementation
├── configs/
│   └── config.example.yaml
├── migrations/
│   └── 000001_init_indexer.sql
└── Makefile
```

## How It Works

The indexer engine polls the chain at a configurable interval:

1. Get latest block → compute safe block (`latest - delay_block`)
2. Load cursor per handler → scan events in batches via `eth_getLogs`
3. Dispatch matched logs to your `HandleLogs()` implementation
4. Advance cursor after successful processing

Each handler has its own independent cursor. If handler B fails, handler A's progress is unaffected.

## Implement Your Handler

Edit `internal/handler/handler.go`:

```go
func (h *MyHandler) Name() string { return "my-handler" }

func (h *MyHandler) Filter() indexer.EventFilter {
    return indexer.EventFilter{
        Addresses: []common.Address{common.HexToAddress("0x...")},
        Topics:    [][]common.Hash{{crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))}},
    }
}

func (h *MyHandler) HandleLogs(ctx context.Context, logs []types.Log) error {
    // Your business logic here.
    // MUST be idempotent — the engine provides at-least-once delivery.
    return nil
}
```

## Configuration

### YAML

See `configs/config.example.yaml` for all available options.

### Environment Variables

Environment variables override YAML values. Prefix: `INDEXER_`.

| Variable | Description |
|----------|-------------|
| `INDEXER_RPC_URL` | Chain RPC endpoint |
| `INDEXER_CHAIN_ID` | Chain ID |
| `INDEXER_POLL_INTERVAL` | Poll interval (e.g. `3s`) |
| `INDEXER_DELAY_BLOCK` | Safe block delay |
| `INDEXER_START_BLOCK` | Block to start scanning from |
| `INDEXER_POSTGRES_HOST` | Postgres host |
| `INDEXER_POSTGRES_PORT` | Postgres port |
| `INDEXER_POSTGRES_DBNAME` | Database name |
| `INDEXER_POSTGRES_USERNAME` | Database user |
| `INDEXER_POSTGRES_PASSWORD` | Database password |

## Make Targets

| Target | Description |
|--------|-------------|
| `make indexer` | Run the indexer |
| `make build` | Build binaries to `bin/` |
| `make dbgen` | Generate GORM models and queries |
| `make migrate-up` | Run database migrations (requires [goose](https://github.com/pressly/goose)) |
| `make migrate-down` | Rollback last migration |
| `make migrate-status` | Show migration status |
| `make test` | Run tests |
| `make clean` | Remove build artifacts |

## Key Design Decisions

- **At-least-once delivery**: handlers may receive the same logs on retry. Implement idempotency using unique constraints or deduplication by `(txHash, logIndex)`.
- **Reorg strategy**: relies on `delay_block` to avoid short-lived reorgs. No block hash verification. If your business is reorg-sensitive, add your own checks.
- **Per-handler cursors**: each handler tracks its own progress independently via the `scan_cursor` table.
- **Fail fast**: empty filters, missing config fields, and invalid chain IDs are rejected at startup.
