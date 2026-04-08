package indexer

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// --- in-memory CursorStore for testing ---

type memoryCursorStore struct {
	mu      sync.Mutex
	cursors map[string]uint64
}

func newMemoryCursorStore() *memoryCursorStore {
	return &memoryCursorStore{cursors: make(map[string]uint64)}
}

func (m *memoryCursorStore) GetCursor(_ context.Context, name string) (uint64, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.cursors[name]
	return v, ok, nil
}

func (m *memoryCursorStore) UpsertCursor(_ context.Context, name string, block uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cursors[name] = block
	return nil
}

// --- mock handler ---

type mockHandler struct {
	name      string
	filter    EventFilter
	logs      []types.Log
	handleErr error
}

func (h *mockHandler) Name() string           { return h.name }
func (h *mockHandler) Filter() EventFilter     { return h.filter }
func (h *mockHandler) HandleLogs(_ context.Context, logs []types.Log) error {
	h.logs = append(h.logs, logs...)
	return h.handleErr
}

func validFilter() EventFilter {
	return EventFilter{
		Addresses: []common.Address{common.HexToAddress("0x1")},
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// --- computeSafeBlock tests ---

func TestComputeSafeBlock(t *testing.T) {
	tests := []struct {
		latest, delay uint64
		wantSafe      uint64
		wantOk        bool
	}{
		{100, 3, 97, true},
		{3, 3, 0, false},
		{0, 3, 0, false},
		{10, 0, 10, true},
		{1, 5, 0, false},
	}
	for _, tt := range tests {
		safe, ok := computeSafeBlock(tt.latest, tt.delay)
		if safe != tt.wantSafe || ok != tt.wantOk {
			t.Errorf("computeSafeBlock(%d, %d) = (%d, %v), want (%d, %v)",
				tt.latest, tt.delay, safe, ok, tt.wantSafe, tt.wantOk)
		}
	}
}

// --- processHandler: StartBlock in the future → no cursor written ---

func TestProcessHandler_StartBlockInFuture(t *testing.T) {
	store := newMemoryCursorStore()
	e := &Engine{
		cfg:    Config{StartBlock: 200, LogScanBatchBlocks: 500}.withDefaults(),
		cursor: store,
		logger: testLogger(),
	}
	h := &mockHandler{name: "test", filter: validFilter()}

	// safe=100, StartBlock=200 → should NOT persist cursor
	err := e.processHandler(context.Background(), h, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, exists, _ := store.GetCursor(context.Background(), "test")
	if exists {
		t.Error("cursor should NOT exist when StartBlock > safe")
	}
}

// --- processHandler: StartBlock=0 → cursor set to safe ---

func TestProcessHandler_NoStartBlock(t *testing.T) {
	store := newMemoryCursorStore()
	e := &Engine{
		cfg:    Config{StartBlock: 0, LogScanBatchBlocks: 500}.withDefaults(),
		cursor: store,
		logger: testLogger(),
	}
	h := &mockHandler{name: "test", filter: validFilter()}

	// safe=100, StartBlock=0 → cursor set to safe (100), no scan needed
	err := e.processHandler(context.Background(), h, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cursor, exists, _ := store.GetCursor(context.Background(), "test")
	if !exists || cursor != 100 {
		t.Errorf("cursor = %d, exists = %v, want 100, true", cursor, exists)
	}
}

// --- processHandler: cursor already at safe → no scan ---

func TestProcessHandler_CursorAlreadyCaughtUp(t *testing.T) {
	store := newMemoryCursorStore()
	_ = store.UpsertCursor(context.Background(), "test", 100)

	e := &Engine{
		cfg:    Config{StartBlock: 1, LogScanBatchBlocks: 500}.withDefaults(),
		cursor: store,
		logger: testLogger(),
	}
	h := &mockHandler{name: "test", filter: validFilter()}

	// cursor=100, safe=100 → nothing to do
	err := e.processHandler(context.Background(), h, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cursor, _, _ := store.GetCursor(context.Background(), "test")
	if cursor != 100 {
		t.Errorf("cursor = %d, want 100 (unchanged)", cursor)
	}
}

// --- multi handler: independent cursors ---

func TestProcessHandler_IndependentCursors(t *testing.T) {
	store := newMemoryCursorStore()
	e := &Engine{
		cfg:    Config{StartBlock: 0, LogScanBatchBlocks: 500}.withDefaults(),
		cursor: store,
		logger: testLogger(),
	}

	hA := &mockHandler{name: "handler-a", filter: validFilter()}
	hB := &mockHandler{name: "handler-b", filter: validFilter()}

	// StartBlock=0, safe=50 → both cursors initialized to safe (50)
	_ = e.processHandler(context.Background(), hA, 50)
	_ = e.processHandler(context.Background(), hB, 50)

	cursorA, existsA, _ := store.GetCursor(context.Background(), "handler-a")
	cursorB, existsB, _ := store.GetCursor(context.Background(), "handler-b")

	if !existsA || cursorA != 50 {
		t.Errorf("handler-a cursor = %d, exists = %v, want 50, true", cursorA, existsA)
	}
	if !existsB || cursorB != 50 {
		t.Errorf("handler-b cursor = %d, exists = %v, want 50, true", cursorB, existsB)
	}

	// Advance handler-a manually, handler-b stays
	_ = store.UpsertCursor(context.Background(), "handler-a", 100)

	cursorA, _, _ = store.GetCursor(context.Background(), "handler-a")
	cursorB, _, _ = store.GetCursor(context.Background(), "handler-b")

	if cursorA != 100 {
		t.Errorf("handler-a cursor = %d, want 100", cursorA)
	}
	if cursorB != 50 {
		t.Errorf("handler-b cursor = %d, want 50 (should not have changed)", cursorB)
	}
}

// --- Config validation tests ---

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"valid", Config{RPCURL: "http://localhost:8545", ChainID: 1}, false},
		{"missing rpc_url", Config{RPCURL: "", ChainID: 1}, true},
		{"missing chain_id", Config{RPCURL: "http://localhost:8545", ChainID: 0}, true},
		{"negative chain_id", Config{RPCURL: "http://localhost:8545", ChainID: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigWithDefaults(t *testing.T) {
	cfg := Config{}.withDefaults()
	if cfg.PollInterval <= 0 {
		t.Error("PollInterval should have a default")
	}
	if cfg.DelayBlock == 0 {
		t.Error("DelayBlock should have a default")
	}
	if cfg.LogScanBatchBlocks == 0 {
		t.Error("LogScanBatchBlocks should have a default")
	}
}

// --- New() validation tests ---

func TestNew_RejectsNoHandlers(t *testing.T) {
	cfg := Config{RPCURL: "http://localhost:8545", ChainID: 1}
	_, err := New(cfg, newMemoryCursorStore(), testLogger())
	if err == nil {
		t.Error("expected error for no handlers")
	}
}

func TestNew_RejectsEmptyFilter(t *testing.T) {
	cfg := Config{RPCURL: "http://localhost:8545", ChainID: 1}
	h := &mockHandler{name: "empty", filter: EventFilter{}}
	_, err := New(cfg, newMemoryCursorStore(), testLogger(), h)
	if err == nil {
		t.Error("expected error for empty filter")
	}
}

func TestNew_RejectsDuplicateNames(t *testing.T) {
	cfg := Config{RPCURL: "http://localhost:8545", ChainID: 1}
	h1 := &mockHandler{name: "dup", filter: validFilter()}
	h2 := &mockHandler{name: "dup", filter: validFilter()}
	_, err := New(cfg, newMemoryCursorStore(), testLogger(), h1, h2)
	if err == nil {
		t.Error("expected error for duplicate handler names")
	}
}

func TestNew_RejectsInvalidConfig(t *testing.T) {
	h := &mockHandler{name: "test", filter: validFilter()}
	_, err := New(Config{}, newMemoryCursorStore(), testLogger(), h)
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

// --- memoryCursorStore tests ---

func TestMemoryCursorStore_ReadWrite(t *testing.T) {
	store := newMemoryCursorStore()
	ctx := context.Background()

	_, exists, err := store.GetCursor(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("expected cursor not to exist initially")
	}

	if err := store.UpsertCursor(ctx, "test", 42); err != nil {
		t.Fatal(err)
	}

	block, exists, err := store.GetCursor(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if !exists || block != 42 {
		t.Errorf("got block=%d exists=%v, want 42/true", block, exists)
	}

	if err := store.UpsertCursor(ctx, "test", 100); err != nil {
		t.Fatal(err)
	}
	block, _, _ = store.GetCursor(ctx, "test")
	if block != 100 {
		t.Errorf("got block=%d, want 100", block)
	}

	_, exists, _ = store.GetCursor(ctx, "other")
	if exists {
		t.Error("different name should not exist")
	}
}
