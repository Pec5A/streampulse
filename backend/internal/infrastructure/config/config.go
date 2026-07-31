// Package config loads all runtime configuration from environment
// variables (12-Factor App) — no hardcoded values, no config files.
package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	Port          string
	DatabaseURL   string
	JWTSecret     string
	JWTExpiration time.Duration
	Environment   string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:          getEnv("PORT", "8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		JWTExpiration: 24 * time.Hour,
		Environment:   getEnv("ENVIRONMENT", "development"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET is required and must be at least 32 characters")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
