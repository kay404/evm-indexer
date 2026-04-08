package postgres

import (
	"log"
	"os"
	"time"

	glogger "gorm.io/gorm/logger"
)

// Options holds optional settings for NewDB.
type Options struct {
	maxOpenConns    int
	maxIdleConns    int
	connMaxLifetime time.Duration
	gormLogger      glogger.Interface
	otelServiceName string
}

// Option configures a database connection.
type Option func(*Options)

// WithPool sets connection pool parameters.
func WithPool(maxOpen, maxIdle int, maxLifetime time.Duration) Option {
	return func(o *Options) {
		o.maxOpenConns = maxOpen
		o.maxIdleConns = maxIdle
		o.connMaxLifetime = maxLifetime
	}
}

// WithLogger enables GORM SQL logging.
// When showSQL is true, log level is Info; otherwise Silent.
func WithLogger(showSQL bool) Option {
	return func(o *Options) {
		level := glogger.Silent
		if showSQL {
			level = glogger.Info
		}
		o.gormLogger = glogger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			glogger.Config{
				SlowThreshold:             500 * time.Millisecond,
				LogLevel:                  level,
				IgnoreRecordNotFoundError: true,
				Colorful:                  false,
			},
		)
	}
}

// WithOtel enables OpenTelemetry tracing on the database connection.
// The serviceName is used as a tracing attribute.
func WithOtel(serviceName string) Option {
	return func(o *Options) {
		o.otelServiceName = serviceName
	}
}
