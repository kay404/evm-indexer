package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kay404/evm-indexer/internal/config"
	"github.com/kay404/evm-indexer/internal/handler"
	"github.com/kay404/evm-indexer/internal/indexer"
	"github.com/kay404/evm-indexer/internal/mysql"
	"github.com/kay404/evm-indexer/internal/postgres"

	"gorm.io/gorm"
)

type AppConfig struct {
	Databases struct {
		Driver   string `yaml:"driver"`
		Postgres struct {
			Default config.PostgresConfig `yaml:"default"`
		} `yaml:"postgres"`
		MySQL struct {
			Default config.MySQLConfig `yaml:"default"`
		} `yaml:"mysql"`
	} `yaml:"databases"`
	Indexer   indexer.Config           `yaml:"indexer"`
	Contracts []handler.ContractConfig `yaml:"contracts"`
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "configs/config.yaml", "path to yaml config file")
	flag.Parse()

	var cfg AppConfig
	if err := config.Load(&cfg, configPath); err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	// Environment variables override config file values.
	cfg.Indexer.ApplyEnv("INDEXER")
	config.SetString(&cfg.Databases.Driver, "INDEXER_DB_DRIVER")
	cfg.Databases.Postgres.Default.ApplyEnv("INDEXER")
	cfg.Databases.MySQL.Default.ApplyEnv("INDEXER")

	// Default driver to postgres for backward compatibility.
	if cfg.Databases.Driver == "" {
		cfg.Databases.Driver = "postgres"
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if len(cfg.Contracts) == 0 {
		logger.Error("no contracts configured — add at least one entry to 'contracts' in config")
		os.Exit(1)
	}

	db, err := openDB(cfg)
	if err != nil {
		logger.Error("connect database failed", "driver", cfg.Databases.Driver, "error", err)
		os.Exit(1)
	}

	cursor := indexer.NewGormCursorStore(db)

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
		"db_driver", cfg.Databases.Driver,
		"handlers", len(handlers),
	)
	if err := engine.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("indexer stopped with error", "error", err)
		os.Exit(1)
	}
	logger.Info("indexer stopped")
}

func openDB(cfg AppConfig) (*gorm.DB, error) {
	switch cfg.Databases.Driver {
	case "postgres":
		return postgres.NewDB(cfg.Databases.Postgres.Default)
	case "mysql":
		return mysql.NewDB(cfg.Databases.MySQL.Default)
	default:
		return nil, fmt.Errorf("unsupported database driver: %q (supported: postgres, mysql)", cfg.Databases.Driver)
	}
}
