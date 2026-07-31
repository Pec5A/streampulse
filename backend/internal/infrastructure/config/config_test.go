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
