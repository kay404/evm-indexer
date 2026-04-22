package main

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/kay404/evm-indexer/internal/config"
	"github.com/kay404/evm-indexer/internal/dbgen"
	"github.com/kay404/evm-indexer/internal/mysql"
	"github.com/kay404/evm-indexer/internal/postgres"

	"gorm.io/gorm"
)

func main() {
	var (
		configPath  string
		outPath     string
		modelPkg    string
		tables      string
		stripPrefix string
	)

	flag.StringVar(&configPath, "config", "configs/config.yaml", "path to yaml config file")
	flag.StringVar(&outPath, "out", "internal/db/query", "output path for generated code")
	flag.StringVar(&modelPkg, "model-pkg", "model", "model package name")
	flag.StringVar(&tables, "tables", "", "comma-separated table names (empty = all)")
	flag.StringVar(&stripPrefix, "strip-prefix", "", "prefix to strip from table names (e.g. t_)")
	flag.Parse()

	var cfg struct {
		Databases struct {
			Driver   string `yaml:"driver"`
			Postgres struct {
				Default config.PostgresConfig `yaml:"default"`
			} `yaml:"postgres"`
			MySQL struct {
				Default config.MySQLConfig `yaml:"default"`
			} `yaml:"mysql"`
		} `yaml:"databases"`
	}
	if err := config.Load(&cfg, configPath); err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Environment variables override config file values.
	config.SetString(&cfg.Databases.Driver, "INDEXER_DB_DRIVER")
	cfg.Databases.Postgres.Default.ApplyEnv("INDEXER")
	cfg.Databases.MySQL.Default.ApplyEnv("INDEXER")

	if cfg.Databases.Driver == "" {
		cfg.Databases.Driver = "postgres"
	}

	db, err := openDB(cfg.Databases.Driver, cfg)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}

	var tableList []string
	if tables != "" {
		tableList = strings.Split(tables, ",")
	}

	if err := dbgen.Run(db, dbgen.Config{
		OutPath:      outPath,
		ModelPkgPath: modelPkg,
		Tables:       tableList,
		StripPrefix:  stripPrefix,
	}); err != nil {
		log.Fatalf("dbgen: %v", err)
	}
}

func openDB(driver string, cfg struct {
	Databases struct {
		Driver   string `yaml:"driver"`
		Postgres struct {
			Default config.PostgresConfig `yaml:"default"`
		} `yaml:"postgres"`
		MySQL struct {
			Default config.MySQLConfig `yaml:"default"`
		} `yaml:"mysql"`
	} `yaml:"databases"`
},
) (*gorm.DB, error) {
	switch driver {
	case "postgres":
		return postgres.NewDB(cfg.Databases.Postgres.Default)
	case "mysql":
		return mysql.NewDB(cfg.Databases.MySQL.Default)
	default:
		return nil, fmt.Errorf("unsupported database driver: %q (supported: postgres, mysql)", driver)
	}
}
