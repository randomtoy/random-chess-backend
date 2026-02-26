package config

import "os"

// Config holds application configuration read from environment variables.
type Config struct {
	Port        string
	DatabaseURL string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return &Config{
		Port:        port,
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}
}
