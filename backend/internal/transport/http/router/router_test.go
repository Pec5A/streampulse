package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/streampulse/backend/internal/infrastructure/auth"
	"github.com/streampulse/backend/internal/transport/http/handler"
)

func TestRouter_HealthEndpoint(t *testing.T) {
	jwtManager := auth.NewJWTManager("test-secret-at-least-32-bytes-long", time.Hour)
	mux := New(Handlers{Auth: handler.NewAuthHandler(nil)}, jwtManager)

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
	mux := New(Handlers{Auth: handler.NewAuthHandler(nil)}, jwtManager)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (refresh must require auth)", rec.Code, http.StatusUnauthorized)
	}
}
