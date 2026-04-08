package config

import "testing"

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
