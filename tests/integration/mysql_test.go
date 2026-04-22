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
	iMySQL "github.com/kay404/evm-indexer/internal/mysql"
)

// startMySQL spins up a disposable MySQL container and returns a raw *sql.DB
// plus the MySQLConfig that points at it. The container is cleaned up via t.Cleanup.
func startMySQL(t *testing.T) (*sql.DB, config.MySQLConfig) {
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

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	host, err := ctr.Host(ctx)
	if err != nil {
		t.Fatalf("container host: %v", err)
	}
	port, err := ctr.MappedPort(ctx, "3306/tcp")
	if err != nil {
		t.Fatalf("container port: %v", err)
	}

	cfg := config.MySQLConfig{
		Host:     host,
		Port:     int(port.Num()),
		DBName:   "testdb",
		Username: "root",
		Password: "testpass",
	}

	return db, cfg
}

// runMigrations applies all MySQL goose migrations.
func runMigrations(t *testing.T, db *sql.DB) {
	t.Helper()
	provider, err := goose.NewProvider(
		goose.DialectMySQL,
		db,
		os.DirFS("../../migrations/mysql"),
	)
	if err != nil {
		t.Fatalf("goose.NewProvider: %v", err)
	}
	results, err := provider.Up(context.Background())
	if err != nil {
		t.Fatalf("goose migrations up: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one migration to run")
	}
	for _, r := range results {
		t.Logf("applied migration: %s", r)
	}
}

func TestMySQLNewDB(t *testing.T) {
	_, cfg := startMySQL(t)

	db, err := iMySQL.NewDB(cfg)
	if err != nil {
		t.Fatalf("mysql.NewDB: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

func TestMySQLMigrations(t *testing.T) {
	rawDB, _ := startMySQL(t)
	runMigrations(t, rawDB)

	// Verify the scan_cursor table exists and has the expected columns.
	rows, err := rawDB.Query("DESCRIBE scan_cursor")
	if err != nil {
		t.Fatalf("DESCRIBE scan_cursor: %v", err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var field, typ, null, key string
		var defVal, extra sql.NullString
		if err := rows.Scan(&field, &typ, &null, &key, &defVal, &extra); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		cols[field] = true
	}

	for _, want := range []string{"name", "last_safe_block_processed", "created_at", "updated_at"} {
		if !cols[want] {
			t.Errorf("expected column %q in scan_cursor, got columns: %v", want, cols)
		}
	}
}
