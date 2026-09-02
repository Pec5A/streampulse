package config

import "testing"

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
