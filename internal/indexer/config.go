package indexer

import (
	"fmt"
	"time"

	"github.com/kay404/evm-indexer/internal/config"
)

// Config holds the indexer engine configuration.
type Config struct {
	// RPCURL is the HTTP RPC endpoint of the chain.
	RPCURL string `yaml:"rpc_url"`

	// ChainID is the chain identifier.
	ChainID int64 `yaml:"chain_id"`

	// PollInterval is how often to poll for new blocks. Default: 3s.
	PollInterval time.Duration `yaml:"poll_interval"`

	// DelayBlock is the number of blocks behind latest to consider "safe". Default: 3.
	DelayBlock uint64 `yaml:"delay_block"`

	// LogScanBatchBlocks is the max block range per eth_getLogs call. Default: 500.
	LogScanBatchBlocks uint64 `yaml:"log_scan_batch_blocks"`

	// StartBlock is the block to start scanning from. 0 means start from current safe block.
	StartBlock uint64 `yaml:"start_block"`

	// VerifyCursorHash enables reorg detection. When true, the engine records the
	// canonical hash of every advanced cursor block and re-verifies it against the
	// chain on each cycle. On mismatch, the cursor is rewound by ReorgRewindDepth
	// blocks and the range is re-scanned (handlers must be idempotent).
	//
	// Default: false (preserves the template's simple no-reorg-check behavior).
	VerifyCursorHash bool `yaml:"verify_cursor_hash"`

	// ReorgRewindDepth is how many blocks to rewind when a reorg is detected.
	// Only used when VerifyCursorHash is true. Default: 10.
	ReorgRewindDepth uint64 `yaml:"reorg_rewind_depth"`

	// HealthAddr is the bind address for the /healthz endpoint (e.g. ":8080").
	// Empty disables the health server.
	HealthAddr string `yaml:"health_addr"`
}

// ApplyEnv overrides Config fields from environment variables.
// Prefix example: "INDEXER" → INDEXER_RPC_URL, INDEXER_CHAIN_ID, etc.
func (c *Config) ApplyEnv(prefix string) {
	p := prefix + "_"
	config.SetString(&c.RPCURL, p+"RPC_URL")
	config.SetInt64(&c.ChainID, p+"CHAIN_ID")
	config.SetDuration(&c.PollInterval, p+"POLL_INTERVAL")
	config.SetUint64(&c.DelayBlock, p+"DELAY_BLOCK")
	config.SetUint64(&c.LogScanBatchBlocks, p+"LOG_SCAN_BATCH_BLOCKS")
	config.SetUint64(&c.StartBlock, p+"START_BLOCK")
	config.SetBool(&c.VerifyCursorHash, p+"VERIFY_CURSOR_HASH")
	config.SetUint64(&c.ReorgRewindDepth, p+"REORG_REWIND_DEPTH")
	config.SetString(&c.HealthAddr, p+"HEALTH_ADDR")
}

// Validate checks that required indexer fields are set.
func (c Config) Validate() error {
	if c.RPCURL == "" {
		return fmt.Errorf("indexer: rpc_url is required")
	}
	if c.ChainID <= 0 {
		return fmt.Errorf("indexer: chain_id must be > 0")
	}
	return nil
}

func (c Config) withDefaults() Config {
	if c.PollInterval <= 0 {
		c.PollInterval = 3 * time.Second
	}
	if c.DelayBlock == 0 {
		c.DelayBlock = 3
	}
	if c.LogScanBatchBlocks == 0 {
		c.LogScanBatchBlocks = 500
	}
	if c.ReorgRewindDepth == 0 {
		c.ReorgRewindDepth = 10
	}
	return c
}
