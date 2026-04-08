package postgres

import (
	"fmt"

	"github.com/kay404/evm-indexer/internal/config"

	"go.opentelemetry.io/otel/attribute"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/plugin/opentelemetry/tracing"
)

// NewDB creates a new GORM database connection from the given PostgresConfig.
func NewDB(cfg config.PostgresConfig, opts ...Option) (*gorm.DB, error) {
	cfg = cfg.Defaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	var o Options
	for _, opt := range opts {
		opt(&o)
	}

	gormCfg := &gorm.Config{}
	if o.gormLogger != nil {
		gormCfg.Logger = o.gormLogger
	} else if cfg.ShowSQL {
		// Respect config ShowSQL when no explicit WithLogger option is given.
		WithLogger(true)(&o)
		gormCfg.Logger = o.gormLogger
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  cfg.DSN(),
		PreferSimpleProtocol: true,
	}), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect postgres: %w", err)
	}

	// OpenTelemetry tracing
	if o.otelServiceName != "" {
		if err := db.Use(tracing.NewPlugin(
			tracing.WithoutServerAddress(),
			tracing.WithAttributes(
				attribute.String("service.name", o.otelServiceName),
				attribute.String("db.system.host", cfg.Host),
			),
			tracing.WithRecordStackTrace(),
		)); err != nil {
			return nil, fmt.Errorf("failed to enable otel tracing: %w", err)
		}
	}

	// Connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}
	if o.maxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(o.maxOpenConns)
	} else if cfg.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if o.maxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(o.maxIdleConns)
	} else if cfg.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if o.connMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(o.connMaxLifetime)
	} else if cfg.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}

	return db, nil
}
