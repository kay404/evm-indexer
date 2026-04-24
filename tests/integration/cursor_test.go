package integration_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"github.com/kay404/evm-indexer/internal/config"
	"github.com/kay404/evm-indexer/internal/indexer"
	iMySQL "github.com/kay404/evm-indexer/internal/mysql"
)

// setupCursorStore starts MySQL, runs migrations, and returns a GormCursorStore.
func setupCursorStore(t *testing.T) *indexer.GormCursorStore {
	t.Helper()
	ctx := context.Background()

	ctr, err := tcmysql.Run(ctx, "mysql:8.0",
		tcmysql.WithDatabase("testdb"),
		tcmysql.WithUsername("root"),
		tcmysql.WithPassword("testpass"),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start mysql container: %v", err)
	}

	dsn, err := ctr.ConnectionString(ctx, "parseTime=true", "multiStatements=true")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	rawDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { rawDB.Close() })

	// Run migrations.
	provider, err := goose.NewProvider(goose.DialectMySQL, rawDB, os.DirFS("../../migrations/mysql"))
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	// Build GORM connection.
	host, err := ctr.Host(ctx)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	port, err := ctr.MappedPort(ctx, "3306/tcp")
	if err != nil {
		t.Fatalf("port: %v", err)
	}

	gormDB, err := iMySQL.NewDB(config.MySQLConfig{
		Host:     host,
		Port:     int(port.Num()),
		DBName:   "testdb",
		Username: "root",
		Password: "testpass",
	})
	if err != nil {
		t.Fatalf("mysql.NewDB: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gormDB.DB(); err == nil {
			sqlDB.Close()
		}
	})

	return indexer.NewGormCursorStore(gormDB)
}

func TestGormCursorStore_MySQL_GetCursor_NotExists(t *testing.T) {
	store := setupCursorStore(t)
	ctx := context.Background()

	block, hash, exists, err := store.GetCursor(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if exists {
		t.Error("expected exists=false for missing cursor")
	}
	if block != 0 {
		t.Errorf("block = %d, want 0", block)
	}
	if hash != (common.Hash{}) {
		t.Errorf("hash = %s, want zero", hash.Hex())
	}
}

func TestGormCursorStore_MySQL_UpsertAndGet(t *testing.T) {
	store := setupCursorStore(t)
	ctx := context.Background()

	// Insert without hash.
	if err := store.UpsertCursor(ctx, "handler-a", 100, common.Hash{}); err != nil {
		t.Fatalf("UpsertCursor (insert): %v", err)
	}
	block, hash, exists, err := store.GetCursor(ctx, "handler-a")
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true after insert")
	}
	if block != 100 {
		t.Errorf("block = %d, want 100", block)
	}
	if hash != (common.Hash{}) {
		t.Errorf("hash = %s, want zero after insert without hash", hash.Hex())
	}

	// Update with a non-zero hash and verify round-trip.
	wantHash := common.HexToHash("0xabc1230000000000000000000000000000000000000000000000000000000def")
	if err := store.UpsertCursor(ctx, "handler-a", 200, wantHash); err != nil {
		t.Fatalf("UpsertCursor (update): %v", err)
	}
	block, hash, _, err = store.GetCursor(ctx, "handler-a")
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if block != 200 {
		t.Errorf("block = %d, want 200", block)
	}
	if hash != wantHash {
		t.Errorf("hash = %s, want %s", hash.Hex(), wantHash.Hex())
	}

	// Upsert back to zero hash clears the stored value.
	if err := store.UpsertCursor(ctx, "handler-a", 300, common.Hash{}); err != nil {
		t.Fatalf("UpsertCursor (clear hash): %v", err)
	}
	_, hash, _, _ = store.GetCursor(ctx, "handler-a")
	if hash != (common.Hash{}) {
		t.Errorf("hash = %s, want zero after clear", hash.Hex())
	}
}

func TestGormCursorStore_MySQL_IndependentCursors(t *testing.T) {
	store := setupCursorStore(t)
	ctx := context.Background()

	if err := store.UpsertCursor(ctx, "handler-x", 10, common.Hash{}); err != nil {
		t.Fatalf("UpsertCursor x: %v", err)
	}
	if err := store.UpsertCursor(ctx, "handler-y", 20, common.Hash{}); err != nil {
		t.Fatalf("UpsertCursor y: %v", err)
	}

	bx, _, _, _ := store.GetCursor(ctx, "handler-x")
	by, _, _, _ := store.GetCursor(ctx, "handler-y")

	if bx != 10 {
		t.Errorf("handler-x block = %d, want 10", bx)
	}
	if by != 20 {
		t.Errorf("handler-y block = %d, want 20", by)
	}
}
