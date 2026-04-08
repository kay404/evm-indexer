package main

import (
	"flag"
	"log"
	"strings"

	"github.com/kay404/evm-indexer/internal/config"
	"github.com/kay404/evm-indexer/internal/dbgen"
	"github.com/kay404/evm-indexer/internal/postgres"
)

func main() {
	var (
		configPath  string
		outPath     string
		modelPkg    string
		tables      string
		stripPrefix string
	)

	flag.StringVar(&configPath, "config", "configs/config.example.yaml", "path to yaml config file")
	flag.StringVar(&outPath, "out", "internal/db/postgres/query", "output path for generated code")
	flag.StringVar(&modelPkg, "model-pkg", "model", "model package name")
	flag.StringVar(&tables, "tables", "", "comma-separated table names (empty = all)")
	flag.StringVar(&stripPrefix, "strip-prefix", "", "prefix to strip from table names (e.g. t_)")
	flag.Parse()

	var cfg struct {
		Databases struct {
			Postgres struct {
				Default config.PostgresConfig `yaml:"default"`
			} `yaml:"postgres"`
		} `yaml:"databases"`
	}
	if err := config.Load(&cfg, configPath); err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Environment variables override config file values.
	cfg.Databases.Postgres.Default.ApplyEnv("INDEXER")

	db, err := postgres.NewDB(cfg.Databases.Postgres.Default)
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
