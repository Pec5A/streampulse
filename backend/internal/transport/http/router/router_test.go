package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/streampulse/backend/internal/infrastructure/auth"
	"github.com/streampulse/backend/internal/transport/http/handler"
)

func TestRouter_HealthEndpoint(t *testing.T) {
	jwtManager := auth.NewJWTManager("test-secret-at-least-32-bytes-long", time.Hour)
	mux := New(Handlers{Auth: handler.NewAuthHandler(nil), User: handler.NewUserHandler(nil)}, jwtManager)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != `{"status":"ok"}`+"\n" {
		t.Fatalf("body = %q, want %q", got, `{"status":"ok"}`+"\n")
	}
}

func TestRouter_RefreshRequiresAuth(t *testing.T) {
	jwtManager := auth.NewJWTManager("test-secret-at-least-32-bytes-long", time.Hour)
	mux := New(Handlers{Auth: handler.NewAuthHandler(nil), User: handler.NewUserHandler(nil)}, jwtManager)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (refresh must require auth)", rec.Code, http.StatusUnauthorized)
	}
}

func TestRouter_RGPDRoutesRequireAuth(t *testing.T) {
	jwtManager := auth.NewJWTManager("test-secret-at-least-32-bytes-long", time.Hour)
	mux := New(Handlers{Auth: handler.NewAuthHandler(nil), User: handler.NewUserHandler(nil)}, jwtManager)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/users/me"},
		{http.MethodGet, "/api/v1/users/me/data"},
		{http.MethodDelete, "/api/v1/users/me"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want %d", tc.method, tc.path, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestHealth_ReportsBuildInfoWhenStamped(t *testing.T) {
	mux := New(Handlers{
		Build: BuildInfo{Version: "1.4.2", Commit: "abc1234"},
		Auth:  handler.NewAuthHandler(nil),
	}, auth.NewJWTManager("test-secret-at-least-32-bytes-long", time.Hour))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode /health: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
	if body["version"] != "1.4.2" {
		t.Errorf("version = %q, want 1.4.2 — a rollback decision needs this without SSHing in", body["version"])
	}
	if body["commit"] != "abc1234" {
		t.Errorf("commit = %q, want abc1234", body["commit"])
	}
}

func TestHealth_OmitsBuildInfoOnAnUnstampedBuild(t *testing.T) {
	// An empty version in a probe response reads as "deployed unknown";
	// absent is honest.
	mux := New(Handlers{Auth: handler.NewAuthHandler(nil)}, auth.NewJWTManager("test-secret-at-least-32-bytes-long", time.Hour))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode /health: %v", err)
	}
	if _, ok := body["version"]; ok {
		t.Errorf("version present on an unstamped build: %v", body)
	}
}
