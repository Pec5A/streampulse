// Package config loads all runtime configuration from environment
// variables (12-Factor App) — no hardcoded values, no config files.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port          string
	DatabaseURL   string
	JWTSecret     string
	JWTExpiration time.Duration
	Environment   string

	// --- Tracing (OpenTelemetry) ---

	ServiceName    string
	ServiceVersion string
	// OTLPEndpoint is the collector address (host:port). Empty disables
	// tracing, which is the default: the API must stay runnable on a laptop
	// with nothing but a Postgres, and a hard dependency on a collector would
	// make the observability stack a prerequisite for every developer.
	OTLPEndpoint string
	// TraceSampleRatio is head sampling, 0..1.
	TraceSampleRatio float64
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:          getEnv("PORT", "8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		JWTSecret:     os.Getenv("JWT_SECRET"),
		JWTExpiration: 24 * time.Hour,
		Environment:   getEnv("ENVIRONMENT", "development"),

		ServiceName:    getEnv("OTEL_SERVICE_NAME", "streampulse-api"),
		ServiceVersion: getEnv("SERVICE_VERSION", "dev"),
		OTLPEndpoint:   os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	}

	ratio, err := parseRatio(getEnv("OTEL_TRACES_SAMPLER_ARG", "1.0"))
	if err != nil {
		return nil, err
	}
	cfg.TraceSampleRatio = ratio

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET is required and must be at least 32 characters")
	}

	return cfg, nil
}

func parseRatio(raw string) (float64, error) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("OTEL_TRACES_SAMPLER_ARG must be a number between 0 and 1: %w", err)
	}
	if v < 0 || v > 1 {
		return 0, fmt.Errorf("OTEL_TRACES_SAMPLER_ARG must be between 0 and 1, got %v", v)
	}
	return v, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
