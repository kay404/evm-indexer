package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kay404/evm-indexer/internal/config"
	"github.com/kay404/evm-indexer/internal/handler"
	"github.com/kay404/evm-indexer/internal/indexer"
	"github.com/kay404/evm-indexer/internal/postgres"
)

type AppConfig struct {
	Databases struct {
		Postgres struct {
			Default config.PostgresConfig `yaml:"default"`
		} `yaml:"postgres"`
	} `yaml:"databases"`
	Indexer indexer.Config `yaml:"indexer"`
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "configs/config.example.yaml", "path to yaml config file")
	flag.Parse()

	var cfg AppConfig
	if err := config.Load(&cfg, configPath); err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	// Environment variables override config file values.
	// e.g. INDEXER_RPC_URL, INDEXER_CHAIN_ID, INDEXER_POSTGRES_HOST, etc.
	cfg.Indexer.ApplyEnv("INDEXER")
	cfg.Databases.Postgres.Default.ApplyEnv("INDEXER")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	db, err := postgres.NewDB(cfg.Databases.Postgres.Default)
	if err != nil {
		logger.Error("connect database failed", "error", err)
		os.Exit(1)
	}

	cursor := indexer.NewPostgresCursorStore(db)

	// Register handler(s) — edit internal/handler/ to add your business logic.
	h := &handler.LogHandler{Logger: logger}

	engine, err := indexer.New(cfg.Indexer, cursor, logger, h)
	if err != nil {
		logger.Error("create indexer failed", "error", err)
		os.Exit(1)
	}
	// Register DB for graceful close on shutdown.
	if sqlDB, err := db.DB(); err == nil {
		engine.AddCloser(sqlDB)
	}
	defer engine.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("indexer started", "rpc", cfg.Indexer.RPCURL, "chain_id", cfg.Indexer.ChainID)
	if err := engine.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("indexer stopped with error", "error", err)
		os.Exit(1)
	}
	logger.Info("indexer stopped")
}
