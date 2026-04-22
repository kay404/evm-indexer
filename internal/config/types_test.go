package config

import (
	"os"
	"testing"
)

func TestPostgresConfig_DSN(t *testing.T) {
	c := PostgresConfig{
		Host:     "localhost",
		Port:     5432,
		DBName:   "testdb",
		Username: "user",
		Password: "pass",
		SSLMode:  "require",
	}
	want := "host=localhost user=user password=pass dbname=testdb port=5432 sslmode=require TimeZone=UTC"
	if got := c.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

func TestPostgresConfig_DSN_DefaultSSLMode(t *testing.T) {
	c := PostgresConfig{Host: "h", Port: 5432, DBName: "d", Username: "u"}
	dsn := c.DSN()
	if dsn == "" {
		t.Fatal("DSN should not be empty")
	}
	// SSLMode defaults to "disable" when empty
	if got := c.DSN(); got != "host=h user=u password= dbname=d port=5432 sslmode=disable TimeZone=UTC" {
		t.Errorf("DSN() = %q", got)
	}
}

func TestPostgresConfig_Validate(t *testing.T) {
	valid := PostgresConfig{Host: "localhost", Port: 5432, DBName: "test", Username: "user"}

	if err := valid.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	tests := []struct {
		name   string
		modify func(*PostgresConfig)
	}{
		{"missing host", func(c *PostgresConfig) { c.Host = "" }},
		{"missing dbname", func(c *PostgresConfig) { c.DBName = "" }},
		{"missing username", func(c *PostgresConfig) { c.Username = "" }},
		{"zero port", func(c *PostgresConfig) { c.Port = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid
			tt.modify(&c)
			if err := c.Validate(); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestPostgresConfig_Defaults(t *testing.T) {
	c := PostgresConfig{}.Defaults()
	if c.Port != 5432 {
		t.Errorf("Port = %d, want 5432", c.Port)
	}
	if c.SSLMode != "disable" {
		t.Errorf("SSLMode = %q, want disable", c.SSLMode)
	}
}

// --- MySQL tests ---

func TestMySQLConfig_DSN(t *testing.T) {
	c := MySQLConfig{
		Host:     "localhost",
		Port:     3306,
		DBName:   "testdb",
		Username: "root",
		Password: "secret",
		Charset:  "utf8mb4",
		Loc:      "UTC",
	}
	want := "root:secret@tcp(localhost:3306)/testdb?charset=utf8mb4&parseTime=true&loc=UTC"
	if got := c.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

func TestMySQLConfig_DSN_LocEscaping(t *testing.T) {
	c := MySQLConfig{
		Host:     "localhost",
		Port:     3306,
		DBName:   "testdb",
		Username: "root",
		Password: "pass",
		Loc:      "Asia/Shanghai",
	}
	want := "root:pass@tcp(localhost:3306)/testdb?charset=utf8mb4&parseTime=true&loc=Asia%2FShanghai"
	if got := c.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

func TestMySQLConfig_DSN_Defaults(t *testing.T) {
	c := MySQLConfig{Host: "h", Port: 3306, DBName: "d", Username: "u"}
	// charset defaults to utf8mb4, loc defaults to UTC
	want := "u:@tcp(h:3306)/d?charset=utf8mb4&parseTime=true&loc=UTC"
	if got := c.DSN(); got != want {
		t.Errorf("DSN() = %q, want %q", got, want)
	}
}

func TestMySQLConfig_Validate(t *testing.T) {
	valid := MySQLConfig{Host: "localhost", Port: 3306, DBName: "test", Username: "root"}

	if err := valid.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	tests := []struct {
		name   string
		modify func(*MySQLConfig)
	}{
		{"missing host", func(c *MySQLConfig) { c.Host = "" }},
		{"missing dbname", func(c *MySQLConfig) { c.DBName = "" }},
		{"missing username", func(c *MySQLConfig) { c.Username = "" }},
		{"zero port", func(c *MySQLConfig) { c.Port = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid
			tt.modify(&c)
			if err := c.Validate(); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestMySQLConfig_Defaults(t *testing.T) {
	c := MySQLConfig{}.Defaults()
	if c.Port != 3306 {
		t.Errorf("Port = %d, want 3306", c.Port)
	}
	if c.Charset != "utf8mb4" {
		t.Errorf("Charset = %q, want utf8mb4", c.Charset)
	}
	if c.Loc != "UTC" {
		t.Errorf("Loc = %q, want UTC", c.Loc)
	}
}

func TestMySQLConfig_Defaults_PreservesExistingValues(t *testing.T) {
	c := MySQLConfig{
		Port:    3307,
		Charset: "latin1",
		Loc:     "Asia/Tokyo",
	}.Defaults()
	if c.Port != 3307 {
		t.Errorf("Port = %d, want 3307 (should not override)", c.Port)
	}
	if c.Charset != "latin1" {
		t.Errorf("Charset = %q, want latin1 (should not override)", c.Charset)
	}
	if c.Loc != "Asia/Tokyo" {
		t.Errorf("Loc = %q, want Asia/Tokyo (should not override)", c.Loc)
	}
}

func TestMySQLConfig_ApplyEnv(t *testing.T) {
	os.Setenv("TEST_MYSQL_HOST", "envhost")
	os.Setenv("TEST_MYSQL_PORT", "3307")
	os.Setenv("TEST_MYSQL_DBNAME", "envdb")
	os.Setenv("TEST_MYSQL_USERNAME", "envuser")
	os.Setenv("TEST_MYSQL_PASSWORD", "envpass")
	os.Setenv("TEST_MYSQL_CHARSET", "latin1")
	os.Setenv("TEST_MYSQL_LOC", "Europe/Berlin")
	os.Setenv("TEST_MYSQL_SHOW_SQL", "true")
	defer func() {
		os.Unsetenv("TEST_MYSQL_HOST")
		os.Unsetenv("TEST_MYSQL_PORT")
		os.Unsetenv("TEST_MYSQL_DBNAME")
		os.Unsetenv("TEST_MYSQL_USERNAME")
		os.Unsetenv("TEST_MYSQL_PASSWORD")
		os.Unsetenv("TEST_MYSQL_CHARSET")
		os.Unsetenv("TEST_MYSQL_LOC")
		os.Unsetenv("TEST_MYSQL_SHOW_SQL")
	}()

	var c MySQLConfig
	c.ApplyEnv("TEST")

	if c.Host != "envhost" {
		t.Errorf("Host = %q, want envhost", c.Host)
	}
	if c.Port != 3307 {
		t.Errorf("Port = %d, want 3307", c.Port)
	}
	if c.DBName != "envdb" {
		t.Errorf("DBName = %q, want envdb", c.DBName)
	}
	if c.Username != "envuser" {
		t.Errorf("Username = %q, want envuser", c.Username)
	}
	if c.Password != "envpass" {
		t.Errorf("Password = %q, want envpass", c.Password)
	}
	if c.Charset != "latin1" {
		t.Errorf("Charset = %q, want latin1", c.Charset)
	}
	if c.Loc != "Europe/Berlin" {
		t.Errorf("Loc = %q, want Europe/Berlin", c.Loc)
	}
	if !c.ShowSQL {
		t.Error("ShowSQL should be true")
	}
}
