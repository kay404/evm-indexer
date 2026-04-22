package indexer

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

const (
	rpcMaxRetries     = 3
	rpcInitialBackoff = time.Second
	minBatchBlocks    = uint64(1)
)

// Engine is the core event indexer that polls on-chain logs and dispatches them to handlers.
type Engine struct {
	cfg       Config
	logger    *slog.Logger
	rpcClient *rpc.Client
	ethClient *ethclient.Client
	cursor    CursorStore
	handlers  []EventHandler
	closers   []io.Closer
}

// New creates a new indexer engine. At least one EventHandler must be provided.
func New(cfg Config, cursor CursorStore, logger *slog.Logger, handlers ...EventHandler) (*Engine, error) {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if len(handlers) == 0 {
		return nil, fmt.Errorf("at least one EventHandler is required")
	}

	// Validate handler names are unique and filters are non-empty.
	seen := make(map[string]bool, len(handlers))
	for _, h := range handlers {
		name := h.Name()
		if seen[name] {
			return nil, fmt.Errorf("duplicate handler name: %q", name)
		}
		seen[name] = true

		f := h.Filter()
		if len(f.Addresses) == 0 && len(f.Topics) == 0 {
			return nil, fmt.Errorf("handler %q has empty filter (no addresses and no topics); this would scan all events on chain", name)
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg = cfg.withDefaults()

	rpcClient, err := rpc.Dial(cfg.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("dial rpc: %w", err)
	}

	return &Engine{
		cfg:       cfg,
		logger:    logger,
		rpcClient: rpcClient,
		ethClient: ethclient.NewClient(rpcClient),
		cursor:    cursor,
		handlers:  handlers,
	}, nil
}

// AddCloser registers an io.Closer to be closed when the engine shuts down.
// Use this to attach the database connection or other resources.
func (e *Engine) AddCloser(c io.Closer) {
	e.closers = append(e.closers, c)
}

// Close releases RPC and all registered resources.
func (e *Engine) Close() {
	if e.rpcClient != nil {
		e.rpcClient.Close()
	}
	for _, c := range e.closers {
		_ = c.Close()
	}
}

// Run starts the polling loop. Blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	ticker := time.NewTicker(e.cfg.PollInterval)
	defer ticker.Stop()

	for {
		if err := e.runCycle(ctx); err != nil {
			e.logger.Error("indexer cycle failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (e *Engine) runCycle(ctx context.Context) error {
	latest, err := e.ethClient.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("get latest block: %w", err)
	}

	safe, ok := computeSafeBlock(latest, e.cfg.DelayBlock)
	if !ok {
		return nil
	}

	// Process each handler independently — one handler's failure does not block others.
	for _, h := range e.handlers {
		if err := e.processHandler(ctx, h, safe); err != nil {
			e.logger.Error("handler failed",
				"handler", h.Name(),
				"error", err,
			)
			// Continue with next handler instead of aborting the entire cycle.
		}
	}

	return nil
}

func (e *Engine) processHandler(ctx context.Context, h EventHandler, safe uint64) error {
	name := h.Name()

	cursor, exists, err := e.cursor.GetCursor(ctx, name)
	if err != nil {
		return fmt.Errorf("load cursor [%s]: %w", name, err)
	}
	if !exists {
		if e.cfg.StartBlock > 0 {
			if e.cfg.StartBlock > safe {
				// StartBlock is in the future — don't persist cursor yet, wait for chain to catch up.
				return nil
			}
			cursor = e.cfg.StartBlock - 1
		} else {
			cursor = safe
		}
		if err := e.cursor.UpsertCursor(ctx, name, cursor); err != nil {
			return fmt.Errorf("initialize cursor [%s]: %w", name, err)
		}
	}

	if safe <= cursor {
		return nil
	}

	batch := e.cfg.LogScanBatchBlocks
	from := cursor + 1
	for from <= safe {
		if err := ctx.Err(); err != nil {
			return err
		}

		to := min(from+batch-1, safe)

		logs, err := e.scanWithRetry(ctx, h, from, to)
		if err != nil {
			if isResultTooLargeErr(err) {
				if batch <= minBatchBlocks {
					return fmt.Errorf("handle events [%s] %d-%d: result too large even at min batch: %w", name, from, to, err)
				}
				batch /= 2
				if batch < minBatchBlocks {
					batch = minBatchBlocks
				}
				e.logger.Warn("RPC returned too many results; shrinking batch",
					"handler", name, "new_batch", batch)
				continue
			}
			return fmt.Errorf("handle events [%s] %d-%d: %w", name, from, to, err)
		}

		if len(logs) > 0 {
			e.logger.Info("events found",
				"handler", name,
				"from_block", from,
				"to_block", to,
				"count", len(logs),
			)
			if err := h.HandleLogs(ctx, logs); err != nil {
				return fmt.Errorf("handle events [%s] %d-%d: handler: %w", name, from, to, err)
			}
		}

		if err := e.cursor.UpsertCursor(ctx, name, to); err != nil {
			return fmt.Errorf("update cursor [%s]: %w", name, err)
		}
		from = to + 1

		// Gradually restore batch toward the configured maximum after a shrink.
		if batch < e.cfg.LogScanBatchBlocks {
			batch *= 2
			if batch > e.cfg.LogScanBatchBlocks {
				batch = e.cfg.LogScanBatchBlocks
			}
		}
	}

	return nil
}

// scanWithRetry performs eth_getLogs with exponential backoff on transient RPC errors.
// "Too many results" errors are NOT retried here — the caller shrinks the batch instead.
func (e *Engine) scanWithRetry(ctx context.Context, h EventHandler, from, to uint64) ([]types.Log, error) {
	backoff := rpcInitialBackoff
	var lastErr error
	for attempt := 0; attempt <= rpcMaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		logs, err := e.scan(ctx, h, from, to)
		if err == nil {
			return logs, nil
		}
		if !isTransientErr(err) {
			return nil, err
		}
		lastErr = err
		if attempt == rpcMaxRetries {
			break
		}
		e.logger.Warn("RPC call failed; retrying",
			"handler", h.Name(),
			"from", from,
			"to", to,
			"attempt", attempt+1,
			"backoff", backoff,
			"error", err,
		)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return nil, fmt.Errorf("rpc failed after %d retries: %w", rpcMaxRetries, lastErr)
}

func (e *Engine) scan(ctx context.Context, h EventHandler, from, to uint64) ([]types.Log, error) {
	filter := h.Filter()

	addresses := make([]string, len(filter.Addresses))
	for i, a := range filter.Addresses {
		addresses[i] = a.Hex()
	}

	var topics []any
	for _, level := range filter.Topics {
		if len(level) == 0 {
			topics = append(topics, nil)
		} else if len(level) == 1 {
			topics = append(topics, level[0].Hex())
		} else {
			hexes := make([]string, len(level))
			for i, t := range level {
				hexes[i] = t.Hex()
			}
			topics = append(topics, hexes)
		}
	}

	arg := map[string]any{
		"fromBlock": fmt.Sprintf("0x%x", from),
		"toBlock":   fmt.Sprintf("0x%x", to),
	}
	if len(addresses) > 0 {
		arg["address"] = addresses
	}
	if len(topics) > 0 {
		arg["topics"] = topics
	}

	var logs []types.Log
	if err := e.rpcClient.CallContext(ctx, &logs, "eth_getLogs", arg); err != nil {
		return nil, fmt.Errorf("eth_getLogs: %w", err)
	}
	return logs, nil
}

// isResultTooLargeErr matches common provider error messages indicating the response
// would exceed the node's size limits. Callers should retry with a smaller block range.
// Patterns are kept specific so they don't steal rate-limit errors (see isTransientErr).
func isResultTooLargeErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "too many results") ||
		strings.Contains(msg, "response size") ||
		strings.Contains(msg, "log response size") ||
		strings.Contains(msg, "query returned more than") ||
		strings.Contains(msg, "size exceeded") ||
		strings.Contains(msg, "log limit exceeded") ||
		strings.Contains(msg, "block range") ||
		strings.Contains(msg, "query timeout")
}

// isTransientErr matches errors worth retrying with backoff (rate limits, timeouts,
// connection blips). "Too many results" is a separate case handled by shrinking batches.
func isTransientErr(err error) bool {
	if err == nil || isResultTooLargeErr(err) {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "temporarily unavailable") ||
		strings.Contains(msg, "i/o timeout")
}

func computeSafeBlock(latest, delay uint64) (uint64, bool) {
	if latest <= delay {
		return 0, false
	}
	return latest - delay, true
}
