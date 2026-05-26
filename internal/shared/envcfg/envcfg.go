// Package envcfg provides small typed helpers around os.Getenv with default
// fallbacks, plus a Load function that pulls in a .env file from the current
// working directory if one is present (dev convenience — missing file is not
// an error). All Lumen binaries read config from environment variables; CLI
// flags are intentionally not exposed.
package envcfg

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Load reads a .env file from CWD if present. Missing file is not an error.
// Existing process environment always wins over .env entries.
func Load() {
	_ = godotenv.Load() // silent: dev convenience, .env is optional
}

func String(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func Bool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func Int(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func Duration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
