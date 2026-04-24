package indexer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	rpcMaxRetries     = 3
	rpcInitialBackoff = time.Second
	rpcDialTimeout    = 10 * time.Second
	minBatchBlocks    = uint64(1)
)

// Engine is the core event indexer that polls on-chain logs and dispatches them to handlers.
type Engine struct {
	cfg       Config
	logger    *slog.Logger
	ethClient *ethclient.Client
	cursor    CursorStore
	handlers  []EventHandler
	closers   []io.Closer

	// lastCycleAt holds the UnixNano timestamp of the last successfully-completed
	// indexer cycle. Used by the /healthz endpoint to decide liveness.
	lastCycleAt atomic.Int64
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

	dialCtx, cancel := context.WithTimeout(context.Background(), rpcDialTimeout)
	defer cancel()
	ethClient, err := ethclient.DialContext(dialCtx, cfg.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("dial rpc %s (timeout %s): %w", cfg.RPCURL, rpcDialTimeout, err)
	}

	return &Engine{
		cfg:       cfg,
		logger:    logger,
		ethClient: ethClient,
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
	if e.ethClient != nil {
		e.ethClient.Close()
	}
	for _, c := range e.closers {
		_ = c.Close()
	}
}

// Run starts the polling loop. Blocks until ctx is cancelled.
// Also starts the /healthz HTTP server if cfg.HealthAddr is set.
func (e *Engine) Run(ctx context.Context) error {
	if e.cfg.HealthAddr != "" {
		go e.serveHealth(ctx)
	}

	ticker := time.NewTicker(e.cfg.PollInterval)
	defer ticker.Stop()

	for {
		err := e.runCycle(ctx)
		switch {
		case err == nil:
			e.lastCycleAt.Store(time.Now().UnixNano())
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			// Shutdown path — let the select below exit cleanly.
		default:
			e.logger.Error("indexer cycle failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// serveHealth runs an HTTP server exposing /healthz for container healthchecks.
// /healthz returns 200 if the last successful cycle completed within 3×PollInterval
// (or 10s, whichever is larger), and 503 otherwise. Returns 503 before the first cycle.
func (e *Engine) serveHealth(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", e.healthHandler)
	srv := &http.Server{
		Addr:              e.cfg.HealthAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	e.logger.Info("health server listening", "addr", e.cfg.HealthAddr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		e.logger.Error("health server", "error", err)
	}
}

func (e *Engine) healthHandler(w http.ResponseWriter, _ *http.Request) {
	threshold := max(e.cfg.PollInterval*3, 10*time.Second)
	lastNano := e.lastCycleAt.Load()
	if lastNano == 0 {
		http.Error(w, "warming up: no successful cycle yet\n", http.StatusServiceUnavailable)
		return
	}
	elapsed := time.Since(time.Unix(0, lastNano))
	if elapsed > threshold {
		http.Error(w, fmt.Sprintf("stale: %s since last successful cycle (threshold %s)\n", elapsed, threshold), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "ok: %s since last cycle\n", elapsed.Truncate(time.Millisecond))
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

	cursor, cursorHash, exists, err := e.cursor.GetCursor(ctx, name)
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
		if err := e.cursor.UpsertCursor(ctx, name, cursor, common.Hash{}); err != nil {
			return fmt.Errorf("initialize cursor [%s]: %w", name, err)
		}
		cursorHash = common.Hash{}
	}

	// Reorg check: if enabled and we have a recorded hash, compare it against the chain.
	if e.cfg.VerifyCursorHash && cursorHash != (common.Hash{}) && cursor > 0 {
		actual, err := e.blockHashAt(ctx, cursor)
		if err != nil {
			return fmt.Errorf("reorg check [%s] block %d: %w", name, cursor, err)
		}
		if actual != cursorHash {
			rewound := rewindCursor(cursor, e.cfg.ReorgRewindDepth)
			e.logger.Warn("reorg detected; rewinding cursor",
				"handler", name,
				"block", cursor,
				"expected_hash", cursorHash.Hex(),
				"actual_hash", actual.Hex(),
				"rewound_to", rewound,
			)
			if err := e.cursor.UpsertCursor(ctx, name, rewound, common.Hash{}); err != nil {
				return fmt.Errorf("rewind cursor [%s]: %w", name, err)
			}
			cursor = rewound
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

		// Record the canonical hash of `to` when hash verification is on.
		// Fetching is done AFTER handler success so a handler failure doesn't waste an RPC round-trip.
		var nextHash common.Hash
		if e.cfg.VerifyCursorHash {
			nextHash, err = e.blockHashAt(ctx, to)
			if err != nil {
				return fmt.Errorf("record cursor hash [%s] block %d: %w", name, to, err)
			}
		}

		if err := e.cursor.UpsertCursor(ctx, name, to, nextHash); err != nil {
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

// retryTransient runs fn with exponential backoff on transient RPC errors.
// Non-transient errors are returned immediately so callers can classify them
// (e.g. "too many results" triggers a batch shrink instead of a retry).
func (e *Engine) retryTransient(ctx context.Context, op string, fn func() error) error {
	backoff := rpcInitialBackoff
	var lastErr error
	for attempt := 0; attempt <= rpcMaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := fn()
		if err == nil {
			return nil
		}
		if !isTransientErr(err) {
			return err
		}
		lastErr = err
		if attempt == rpcMaxRetries {
			break
		}
		e.logger.Warn("RPC call failed; retrying",
			"op", op,
			"attempt", attempt+1,
			"backoff", backoff,
			"error", err,
		)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return fmt.Errorf("rpc failed after %d retries: %w", rpcMaxRetries, lastErr)
}

// scanWithRetry performs eth_getLogs with exponential backoff on transient RPC errors.
// "Too many results" errors are NOT retried here — the caller shrinks the batch instead.
func (e *Engine) scanWithRetry(ctx context.Context, h EventHandler, from, to uint64) ([]types.Log, error) {
	var logs []types.Log
	op := fmt.Sprintf("eth_getLogs [%s] %d-%d", h.Name(), from, to)
	err := e.retryTransient(ctx, op, func() error {
		var scanErr error
		logs, scanErr = e.scan(ctx, h, from, to)
		return scanErr
	})
	return logs, err
}

func (e *Engine) scan(ctx context.Context, h EventHandler, from, to uint64) ([]types.Log, error) {
	filter := h.Filter()
	logs, err := e.ethClient.FilterLogs(ctx, ethereum.FilterQuery{
		FromBlock: new(big.Int).SetUint64(from),
		ToBlock:   new(big.Int).SetUint64(to),
		Addresses: filter.Addresses,
		Topics:    filter.Topics,
	})
	if err != nil {
		return nil, fmt.Errorf("eth_getLogs: %w", err)
	}
	return logs, nil
}

// blockHashAt returns the canonical hash for the given block number on the chain,
// retrying transient RPC errors. Used by the reorg-detection path — callers only
// invoke this when cfg.VerifyCursorHash is true.
func (e *Engine) blockHashAt(ctx context.Context, block uint64) (common.Hash, error) {
	var hash common.Hash
	op := fmt.Sprintf("eth_getBlockByNumber block=%d", block)
	err := e.retryTransient(ctx, op, func() error {
		header, hErr := e.ethClient.HeaderByNumber(ctx, new(big.Int).SetUint64(block))
		if hErr != nil {
			return hErr
		}
		hash = header.Hash()
		return nil
	})
	if err != nil {
		return common.Hash{}, fmt.Errorf("header at %d: %w", block, err)
	}
	return hash, nil
}

// rewindCursor returns the block to rewind to when a reorg is detected at `cursor`.
// Kept as a pure function so it can be unit-tested without a live chain.
func rewindCursor(cursor, depth uint64) uint64 {
	if cursor > depth {
		return cursor - depth
	}
	return 0
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
