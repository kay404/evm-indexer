package integration_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

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

	block, exists, err := store.GetCursor(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if exists {
		t.Error("expected exists=false for missing cursor")
	}
	if block != 0 {
		t.Errorf("block = %d, want 0", block)
	}
}

func TestGormCursorStore_MySQL_UpsertAndGet(t *testing.T) {
	store := setupCursorStore(t)
	ctx := context.Background()

	// Insert.
	if err := store.UpsertCursor(ctx, "handler-a", 100); err != nil {
		t.Fatalf("UpsertCursor (insert): %v", err)
	}
	block, exists, err := store.GetCursor(ctx, "handler-a")
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true after insert")
	}
	if block != 100 {
		t.Errorf("block = %d, want 100", block)
	}

	// Update (upsert same name).
	if err := store.UpsertCursor(ctx, "handler-a", 200); err != nil {
		t.Fatalf("UpsertCursor (update): %v", err)
	}
	block, _, err = store.GetCursor(ctx, "handler-a")
	if err != nil {
		t.Fatalf("GetCursor: %v", err)
	}
	if block != 200 {
		t.Errorf("block = %d, want 200", block)
	}
}

func TestGormCursorStore_MySQL_IndependentCursors(t *testing.T) {
	store := setupCursorStore(t)
	ctx := context.Background()

	if err := store.UpsertCursor(ctx, "handler-x", 10); err != nil {
		t.Fatalf("UpsertCursor x: %v", err)
	}
	if err := store.UpsertCursor(ctx, "handler-y", 20); err != nil {
		t.Fatalf("UpsertCursor y: %v", err)
	}

	bx, _, _ := store.GetCursor(ctx, "handler-x")
	by, _, _ := store.GetCursor(ctx, "handler-y")

	if bx != 10 {
		t.Errorf("handler-x block = %d, want 10", bx)
	}
	if by != 20 {
		t.Errorf("handler-y block = %d, want 20", by)
	}
}
