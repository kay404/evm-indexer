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
	Indexer   indexer.Config           `yaml:"indexer"`
	Contracts []handler.ContractConfig `yaml:"contracts"`
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
	cfg.Indexer.ApplyEnv("INDEXER")
	cfg.Databases.Postgres.Default.ApplyEnv("INDEXER")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if len(cfg.Contracts) == 0 {
		logger.Error("no contracts configured — add at least one entry to 'contracts' in config")
		os.Exit(1)
	}

	db, err := postgres.NewDB(cfg.Databases.Postgres.Default)
	if err != nil {
		logger.Error("connect database failed", "error", err)
		os.Exit(1)
	}

	cursor := indexer.NewPostgresCursorStore(db)

	// Build handlers from config.
	var handlers []indexer.EventHandler
	for _, cc := range cfg.Contracts {
		h, err := handler.NewContractHandler(cc, logger)
		if err != nil {
			logger.Error("invalid contract config", "error", err)
			os.Exit(1)
		}
		handlers = append(handlers, h)
	}

	engine, err := indexer.New(cfg.Indexer, cursor, logger, handlers...)
	if err != nil {
		logger.Error("create indexer failed", "error", err)
		os.Exit(1)
	}
	if sqlDB, err := db.DB(); err == nil {
		engine.AddCloser(sqlDB)
	}
	defer engine.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("indexer started",
		"rpc", cfg.Indexer.RPCURL,
		"chain_id", cfg.Indexer.ChainID,
		"handlers", len(handlers),
	)
	if err := engine.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("indexer stopped with error", "error", err)
		os.Exit(1)
	}
	logger.Info("indexer stopped")
}
