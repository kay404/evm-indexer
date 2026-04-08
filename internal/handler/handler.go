// Package handler contains the EventHandler implementation.
// TODO: replace the example LogHandler with your actual business logic.
package handler

import (
	"context"
	"log/slog"

	"github.com/ethereum/go-ethereum/core/types"

	"github.com/kay404/evm-indexer/internal/indexer"
)

// LogHandler is a minimal example handler that logs every event it receives.
// Replace this with your actual business logic.
type LogHandler struct {
	Logger *slog.Logger
}

func (h *LogHandler) Name() string { return "log-handler" }

func (h *LogHandler) Filter() indexer.EventFilter {
	// TODO: replace with your actual contract addresses and event topics.
	// Returning an empty filter causes indexer.New() to return a clear error:
	//   "handler "log-handler" has empty filter ..."
	return indexer.EventFilter{}
}

// HandleLogs processes a batch of matched logs.
// This method MUST be idempotent — the engine provides at-least-once delivery.
// Use unique constraints, upserts, or deduplication by (txHash, logIndex).
func (h *LogHandler) HandleLogs(ctx context.Context, logs []types.Log) error {
	for _, lg := range logs {
		h.Logger.Info("event",
			"address", lg.Address.Hex(),
			"tx", lg.TxHash.Hex(),
			"block", lg.BlockNumber,
			"log_index", lg.Index,
		)
	}
	return nil
}
