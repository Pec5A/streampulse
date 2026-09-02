package config

import (
	"testing"
	"time"
)

func TestLoad_MissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "a-secret-that-is-long-enough-to-pass-32")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for missing DATABASE_URL")
	}
}

func TestLoad_JWTSecretTooShort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "too-short")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for short JWT_SECRET")
	}
}

func TestLoad_Success(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "a-secret-that-is-long-enough-to-pass-32")
	t.Setenv("PORT", "9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
	if cfg.Environment != "development" {
		t.Errorf("Environment = %q, want development (default)", cfg.Environment)
	}
}

func TestLoad_DefaultPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "a-secret-that-is-long-enough-to-pass-32")
	t.Setenv("PORT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want default 8080", cfg.Port)
	}
}

// --- Tracing configuration ---

func TestLoad_TracingDefaultsToOff(t *testing.T) {
	// A developer with only a Postgres running must still get a bootable API:
	// no collector configured means tracing off, not a startup failure.
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "a-secret-that-is-long-enough-to-pass-32")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OTLPEndpoint != "" {
		t.Errorf("OTLPEndpoint = %q, want empty (tracing off)", cfg.OTLPEndpoint)
	}
	if cfg.ServiceName != "streampulse-api" {
		t.Errorf("ServiceName = %q, want streampulse-api", cfg.ServiceName)
	}
	if cfg.TraceSampleRatio != 1.0 {
		t.Errorf("TraceSampleRatio = %v, want 1.0 by default", cfg.TraceSampleRatio)
	}
}

func TestLoad_TracingFromEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "a-secret-that-is-long-enough-to-pass-32")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317")
	t.Setenv("OTEL_SERVICE_NAME", "streampulse-api-staging")
	t.Setenv("SERVICE_VERSION", "1.4.2")
	t.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.25")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OTLPEndpoint != "otel-collector:4317" {
		t.Errorf("OTLPEndpoint = %q", cfg.OTLPEndpoint)
	}
	if cfg.ServiceName != "streampulse-api-staging" {
		t.Errorf("ServiceName = %q", cfg.ServiceName)
	}
	if cfg.ServiceVersion != "1.4.2" {
		t.Errorf("ServiceVersion = %q", cfg.ServiceVersion)
	}
	if cfg.TraceSampleRatio != 0.25 {
		t.Errorf("TraceSampleRatio = %v, want 0.25", cfg.TraceSampleRatio)
	}
}

func TestLoad_RejectsBadSampleRatio(t *testing.T) {
	// Fail at boot rather than silently sampling nothing: a ratio typo that
	// degrades to 0 would leave an apparently healthy service producing no
	// traces at all, discovered only when someone needs one.
	tests := []struct {
		name string
		raw  string
	}{
		{"not a number", "half"},
		{"negative", "-1"},
		{"above one", "2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://localhost/test")
			t.Setenv("JWT_SECRET", "a-secret-that-is-long-enough-to-pass-32")
			t.Setenv("OTEL_TRACES_SAMPLER_ARG", tc.raw)

			if _, err := Load(); err == nil {
				t.Fatalf("Load() with OTEL_TRACES_SAMPLER_ARG=%q error = nil, want a validation error", tc.raw)
			}
		})
	}
}

// --- Durcissement ---

func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("JWT_SECRET", "a-secret-that-is-long-enough-to-pass-32")
}

func TestLoad_JWTExpirationFromEnvironment(t *testing.T) {
	// It was the one hardcoded value left in the file whose whole point is
	// that nothing is — and it decides how long a token outlives the account
	// it names, so an operator must be able to shorten it without a rebuild.
	baseEnv(t)
	t.Setenv("JWT_EXPIRATION", "15m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.JWTExpiration != 15*time.Minute {
		t.Errorf("JWTExpiration = %s, want 15m", cfg.JWTExpiration)
	}
}

func TestLoad_JWTExpirationDefaultsAndValidates(t *testing.T) {
	baseEnv(t)
	t.Setenv("JWT_EXPIRATION", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.JWTExpiration != 24*time.Hour {
		t.Errorf("default JWTExpiration = %s, want 24h", cfg.JWTExpiration)
	}

	for _, bad := range []string{"soon", "-1h", "0"} {
		t.Run(bad, func(t *testing.T) {
			baseEnv(t)
			t.Setenv("JWT_EXPIRATION", bad)
			if _, err := Load(); err == nil {
				t.Errorf("Load() with JWT_EXPIRATION=%q error = nil, want a validation error", bad)
			}
		})
	}
}

func TestLoad_MetricsTokenRequiredOutsideDevelopment(t *testing.T) {
	// Failing at boot is the only way this cannot be forgotten on the day it
	// starts mattering — the day the service becomes publicly reachable.
	baseEnv(t)
	t.Setenv("METRICS_TOKEN", "")
	t.Setenv("ENVIRONMENT", "production")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil in production without METRICS_TOKEN, want a refusal to start")
	}

	t.Setenv("METRICS_TOKEN", "scrape-me")
	if _, err := Load(); err != nil {
		t.Errorf("Load() error = %v with a token set", err)
	}
}

func TestLoad_MetricsTokenOptionalInDevelopment(t *testing.T) {
	baseEnv(t)
	t.Setenv("METRICS_TOKEN", "")
	t.Setenv("ENVIRONMENT", "development")

	if _, err := Load(); err != nil {
		t.Errorf("Load() error = %v; development must stay runnable without a token", err)
	}
}

func TestLoad_CORSAllowlistParsing(t *testing.T) {
	baseEnv(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", " https://a.example , https://b.example ,")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.AllowedOrigins) != 2 {
		t.Fatalf("AllowedOrigins = %v, want 2 entries (the trailing comma must not become an empty origin)", cfg.AllowedOrigins)
	}
	if cfg.AllowedOrigins[0] != "https://a.example" || cfg.AllowedOrigins[1] != "https://b.example" {
		t.Errorf("AllowedOrigins = %v, want them trimmed", cfg.AllowedOrigins)
	}
}

func TestLoad_CORSDefaultsToNoBrowserAccess(t *testing.T) {
	baseEnv(t)
	t.Setenv("CORS_ALLOWED_ORIGINS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.AllowedOrigins) != 0 {
		t.Errorf("AllowedOrigins = %v, want empty: browser access is opt-in", cfg.AllowedOrigins)
	}
}

func TestLoad_AuthRateLimit(t *testing.T) {
	baseEnv(t)
	t.Setenv("AUTH_RATE_LIMIT_PER_MINUTE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AuthRateLimit != 20 {
		t.Errorf("default AuthRateLimit = %d, want 20", cfg.AuthRateLimit)
	}

	baseEnv(t)
	t.Setenv("AUTH_RATE_LIMIT_PER_MINUTE", "-1")
	if _, err := Load(); err == nil {
		t.Error("Load() accepted a negative rate limit")
	}
}
