package indexer

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// EventFilter describes which on-chain events to subscribe to.
type EventFilter struct {
	// Addresses is the list of contract addresses to watch.
	Addresses []common.Address

	// Topics is the event topic filters (topic0, topic1, ...).
	// Each element can be nil to match any value at that position.
	Topics [][]common.Hash
}

// EventHandler is the interface users implement to process on-chain events.
//
// Each handler has its own cursor — if handler B fails, handler A's progress is
// not rolled back. On the next cycle only handler B retries from its own cursor.
//
// The engine provides at-least-once delivery: when a handler returns an error,
// its cursor does NOT advance, and the same logs will be delivered again on retry.
// Implementations MUST be idempotent — use unique constraints, upserts, or
// deduplication by (txHash, logIndex) to safely handle replays.
type EventHandler interface {
	// Name returns a unique, stable identifier for this handler.
	// It is used as the cursor key — changing it resets progress.
	Name() string

	// Filter returns the event filter for this handler.
	Filter() EventFilter

	// HandleLogs is called with a batch of matched logs.
	// Return an error to abort this handler's progress (its cursor will NOT advance).
	// This method may be called again with the same logs on retry — it must be idempotent.
	HandleLogs(ctx context.Context, logs []types.Log) error
}
