package config

import (
	"os"
	"strconv"
)

// Config holds application configuration read from environment variables.
type Config struct {
	Port                string
	DatabaseURL         string
	GameCreateBatchSize int
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	batchSize := 20
	if v := os.Getenv("GAME_CREATE_BATCH_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			batchSize = n
		}
	}

	return &Config{
		Port:                port,
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		GameCreateBatchSize: batchSize,
	}
}
