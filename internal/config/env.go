package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// SetString overrides dst with the value of the environment variable key, if set.
func SetString(dst *string, key string) {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		*dst = v
	}
}

// SetBool overrides dst with the value of the environment variable key, if set.
// Accepts: 1/true/yes/on → true, 0/false/no/off → false.
func SetBool(dst *bool, key string) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		*dst = true
	case "0", "false", "no", "off":
		*dst = false
	}
}

// SetInt overrides dst with the value of the environment variable key, if set.
func SetInt(dst *int, key string) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return
	}
	if n, err := strconv.Atoi(v); err == nil {
		*dst = n
	}
}

// SetInt64 overrides dst with the value of the environment variable key, if set.
func SetInt64(dst *int64, key string) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		*dst = n
	}
}

// SetUint64 overrides dst with the value of the environment variable key, if set.
func SetUint64(dst *uint64, key string) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return
	}
	if n, err := strconv.ParseUint(v, 10, 64); err == nil {
		*dst = n
	}
}

// SetDuration overrides dst with the value of the environment variable key, if set.
// The value is parsed using time.ParseDuration (e.g. "3s", "10m", "1h").
func SetDuration(dst *time.Duration, key string) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return
	}
	if d, err := time.ParseDuration(v); err == nil {
		*dst = d
	}
}
