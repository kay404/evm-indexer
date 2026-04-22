package config

import (
	"fmt"
	"net/url"
	"time"
)

// PostgresConfig is the shared database configuration used across projects.
type PostgresConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	DBName          string        `yaml:"dbname"`
	Username        string        `yaml:"username"`
	Password        string        `yaml:"password"`
	SSLMode         string        `yaml:"sslmode"`
	ShowSQL         bool          `yaml:"showSql"`
	MaxOpenConns    int           `yaml:"maxOpenConns"`
	MaxIdleConns    int           `yaml:"maxIdleConns"`
	ConnMaxLifetime time.Duration `yaml:"connMaxLifetime"`
}

// DSN returns the PostgreSQL connection string.
func (c PostgresConfig) DSN() string {
	sslmode := c.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}
	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=UTC",
		c.Host, c.Username, c.Password, c.DBName, c.Port, sslmode,
	)
}

// ApplyEnv overrides PostgresConfig fields from environment variables.
// Prefix example: "INDEXER" → INDEXER_POSTGRES_HOST, INDEXER_POSTGRES_PORT, etc.
func (c *PostgresConfig) ApplyEnv(prefix string) {
	p := prefix + "_POSTGRES_"
	SetString(&c.Host, p+"HOST")
	SetInt(&c.Port, p+"PORT")
	SetString(&c.DBName, p+"DBNAME")
	SetString(&c.Username, p+"USERNAME")
	SetString(&c.Password, p+"PASSWORD")
	SetString(&c.SSLMode, p+"SSLMODE")
	SetBool(&c.ShowSQL, p+"SHOW_SQL")
	SetInt(&c.MaxOpenConns, p+"MAX_OPEN_CONNS")
	SetInt(&c.MaxIdleConns, p+"MAX_IDLE_CONNS")
	SetDuration(&c.ConnMaxLifetime, p+"CONN_MAX_LIFETIME")
}

// Validate checks that required postgres fields are set.
func (c PostgresConfig) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("postgres: host is required")
	}
	if c.Port <= 0 {
		return fmt.Errorf("postgres: port must be > 0")
	}
	if c.DBName == "" {
		return fmt.Errorf("postgres: dbname is required")
	}
	if c.Username == "" {
		return fmt.Errorf("postgres: username is required")
	}
	return nil
}

// Defaults returns a PostgresConfig with sensible defaults applied.
func (c PostgresConfig) Defaults() PostgresConfig {
	if c.Port == 0 {
		c.Port = 5432
	}
	if c.SSLMode == "" {
		c.SSLMode = "disable"
	}
	return c
}

// MySQLConfig is the database configuration for MySQL/MariaDB.
// Note: parseTime is always enabled (GORM requires it for time.Time scanning).
type MySQLConfig struct {
	Host            string        `yaml:"host"`
	Port            int           `yaml:"port"`
	DBName          string        `yaml:"dbname"`
	Username        string        `yaml:"username"`
	Password        string        `yaml:"password"`
	Charset         string        `yaml:"charset"`
	Loc             string        `yaml:"loc"`
	ShowSQL         bool          `yaml:"showSql"`
	MaxOpenConns    int           `yaml:"maxOpenConns"`
	MaxIdleConns    int           `yaml:"maxIdleConns"`
	ConnMaxLifetime time.Duration `yaml:"connMaxLifetime"`
}

// DSN returns the MySQL connection string in go-sql-driver/mysql DSN format.
// parseTime is always true; loc is URL-escaped to handle values like "Asia/Shanghai".
func (c MySQLConfig) DSN() string {
	charset := c.Charset
	if charset == "" {
		charset = "utf8mb4"
	}
	loc := c.Loc
	if loc == "" {
		loc = "UTC"
	}
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=true&loc=%s",
		c.Username, c.Password, c.Host, c.Port, c.DBName, charset, url.QueryEscape(loc),
	)
}

// ApplyEnv overrides MySQLConfig fields from environment variables.
// Prefix example: "INDEXER" → INDEXER_MYSQL_HOST, INDEXER_MYSQL_PORT, etc.
func (c *MySQLConfig) ApplyEnv(prefix string) {
	p := prefix + "_MYSQL_"
	SetString(&c.Host, p+"HOST")
	SetInt(&c.Port, p+"PORT")
	SetString(&c.DBName, p+"DBNAME")
	SetString(&c.Username, p+"USERNAME")
	SetString(&c.Password, p+"PASSWORD")
	SetString(&c.Charset, p+"CHARSET")
	SetString(&c.Loc, p+"LOC")
	SetBool(&c.ShowSQL, p+"SHOW_SQL")
	SetInt(&c.MaxOpenConns, p+"MAX_OPEN_CONNS")
	SetInt(&c.MaxIdleConns, p+"MAX_IDLE_CONNS")
	SetDuration(&c.ConnMaxLifetime, p+"CONN_MAX_LIFETIME")
}

// Validate checks that required MySQL fields are set.
func (c MySQLConfig) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("mysql: host is required")
	}
	if c.Port <= 0 {
		return fmt.Errorf("mysql: port must be > 0")
	}
	if c.DBName == "" {
		return fmt.Errorf("mysql: dbname is required")
	}
	if c.Username == "" {
		return fmt.Errorf("mysql: username is required")
	}
	return nil
}

// Defaults returns a MySQLConfig with sensible defaults applied.
func (c MySQLConfig) Defaults() MySQLConfig {
	if c.Port == 0 {
		c.Port = 3306
	}
	if c.Charset == "" {
		c.Charset = "utf8mb4"
	}
	if c.Loc == "" {
		c.Loc = "UTC"
	}
	return c
}
