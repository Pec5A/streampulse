// Package config loads all runtime configuration from environment
// variables (12-Factor App) — no hardcoded values, no config files.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port          string
	DatabaseURL   string
	JWTSecret     string
	JWTExpiration time.Duration
	Environment   string
	// StoragePath is where uploaded audio files are written (ticket K2).
	StoragePath string

	// --- Durcissement pré-production ---

	// AllowedOrigins is the CORS allowlist. Empty means no cross-origin
	// access at all, which is the right default for an API whose only client
	// so far is a native app: a browser client is opt-in, never implicit.
	AllowedOrigins []string
	// MetricsToken guards /metrics. Required outside development — see Load.
	MetricsToken string
	// AuthRateLimit is the number of requests per minute allowed per client
	// IP on the authentication endpoints. 0 disables the limit.
	AuthRateLimit int
	// TrustedProxyHops is how many reverse proxies sit in front of the
	// process. 0 when directly exposed, 1 behind a single PaaS load balancer.
	// It decides how far back in X-Forwarded-For the real client is — see
	// middleware.clientIP for why guessing is not an option.
	TrustedProxyHops int

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
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Environment: getEnv("ENVIRONMENT", "development"),
		// Défaut absolu et non "./uploads" : l'étape finale de l'image n'a pas
		// de WORKDIR, donc un chemin relatif résout en /uploads, que
		// l'utilisateur non-root du conteneur ne peut pas créer.
		StoragePath: getEnv("STORAGE_PATH", "/var/lib/streampulse/uploads"),

		AllowedOrigins: splitAndTrim(os.Getenv("CORS_ALLOWED_ORIGINS")),
		MetricsToken:   os.Getenv("METRICS_TOKEN"),

		ServiceName:    getEnv("OTEL_SERVICE_NAME", "streampulse-api"),
		ServiceVersion: getEnv("SERVICE_VERSION", "dev"),
		OTLPEndpoint:   os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	}

	ratio, err := parseRatio(getEnv("OTEL_TRACES_SAMPLER_ARG", "1.0"))
	if err != nil {
		return nil, err
	}
	cfg.TraceSampleRatio = ratio

	// The token lifetime was the one value still hardcoded in a file whose
	// whole point is that nothing is. It also has a security consequence: it
	// is how long a token stays usable after the account it names is deleted,
	// so an operator must be able to shorten it without a rebuild.
	expiration, err := time.ParseDuration(getEnv("JWT_EXPIRATION", "24h"))
	if err != nil {
		return nil, fmt.Errorf("JWT_EXPIRATION must be a Go duration (e.g. 15m, 24h): %w", err)
	}
	if expiration <= 0 {
		return nil, fmt.Errorf("JWT_EXPIRATION must be positive, got %s", expiration)
	}
	cfg.JWTExpiration = expiration

	limit, err := strconv.Atoi(getEnv("AUTH_RATE_LIMIT_PER_MINUTE", "20"))
	if err != nil || limit < 0 {
		return nil, fmt.Errorf("AUTH_RATE_LIMIT_PER_MINUTE must be a non-negative integer")
	}
	cfg.AuthRateLimit = limit

	hops, err := strconv.Atoi(getEnv("TRUSTED_PROXY_HOPS", "0"))
	if err != nil || hops < 0 {
		return nil, fmt.Errorf("TRUSTED_PROXY_HOPS must be a non-negative integer")
	}
	cfg.TrustedProxyHops = hops

	// Refuse to start rather than expose the scrape endpoint. /metrics leaks
	// the route table, traffic volumes and internal topology; the brief names
	// it explicitly among the endpoints to secure. Failing loudly at boot is
	// the only way this cannot be forgotten on the day it starts mattering —
	// which is the day the service becomes publicly reachable.
	if cfg.Environment != "development" && cfg.MetricsToken == "" {
		return nil, fmt.Errorf("METRICS_TOKEN is required when ENVIRONMENT is %q", cfg.Environment)
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("JWT_SECRET is required and must be at least 32 characters")
	}

	return cfg, nil
}

// splitAndTrim turns "a, b" into ["a" "b"], dropping empties so that a
// trailing comma in an env var does not become an empty allowlist entry —
// which would match the empty Origin header.
func splitAndTrim(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
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
