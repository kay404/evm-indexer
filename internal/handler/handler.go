// Package handler contains the EventHandler implementation.
// TODO: add your actual business logic in HandleLogs.
package handler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/kay404/evm-indexer/internal/indexer"
)

// ContractConfig holds the configuration for a single contract handler.
type ContractConfig struct {
	Name    string   `yaml:"name"`
	Address string   `yaml:"address"`
	Events  []string `yaml:"events"`
}

// Validate checks that required fields are set.
func (c ContractConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("contract: name is required")
	}
	if c.Address == "" {
		return fmt.Errorf("contract %q: address is required", c.Name)
	}
	if !common.IsHexAddress(c.Address) {
		return fmt.Errorf("contract %q: address %q is not a valid hex address", c.Name, c.Address)
	}
	if len(c.Events) == 0 {
		return fmt.Errorf("contract %q: at least one event is required", c.Name)
	}
	return nil
}

// ContractHandler monitors a single contract's events based on configuration.
type ContractHandler struct {
	cfg    ContractConfig
	logger *slog.Logger
	filter indexer.EventFilter
}

// NewContractHandler creates a handler from config.
func NewContractHandler(cfg ContractConfig, logger *slog.Logger) (*ContractHandler, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	topics := make([]common.Hash, len(cfg.Events))
	for i, event := range cfg.Events {
		topics[i] = crypto.Keccak256Hash([]byte(event))
	}

	return &ContractHandler{
		cfg:    cfg,
		logger: logger,
		filter: indexer.EventFilter{
			Addresses: []common.Address{common.HexToAddress(cfg.Address)},
			Topics:    [][]common.Hash{topics},
		},
	}, nil
}

func (h *ContractHandler) Name() string { return h.cfg.Name }

func (h *ContractHandler) Filter() indexer.EventFilter { return h.filter }

// HandleLogs processes a batch of matched logs.
// This method MUST be idempotent — the engine provides at-least-once delivery.
// Use unique constraints, upserts, or deduplication by (txHash, logIndex).
func (h *ContractHandler) HandleLogs(ctx context.Context, logs []types.Log) error {
	for _, lg := range logs {
		h.logger.Info("event",
			"handler", h.cfg.Name,
			"address", lg.Address.Hex(),
			"tx", lg.TxHash.Hex(),
			"block", lg.BlockNumber,
			"log_index", lg.Index,
		)
	}
	// TODO: add your business logic here.
	return nil
}
