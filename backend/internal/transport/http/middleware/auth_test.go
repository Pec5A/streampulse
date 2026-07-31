package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/streampulse/backend/internal/infrastructure/auth"
)

func TestRequireAuth_MissingHeader(t *testing.T) {
	jwtManager := auth.NewJWTManager("test-secret-at-least-32-bytes-long", time.Hour)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	handler := RequireAuth(jwtManager)(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_MalformedHeader(t *testing.T) {
	jwtManager := auth.NewJWTManager("test-secret-at-least-32-bytes-long", time.Hour)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	handler := RequireAuth(jwtManager)(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic somevalue")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRequireAuth_ValidTokenInjectsContext(t *testing.T) {
	jwtManager := auth.NewJWTManager("test-secret-at-least-32-bytes-long", time.Hour)
	token, err := jwtManager.Generate("u1", "user")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	var gotID string
	var gotRole string
	var gotOK bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, gotOK = UserID(r.Context())
		gotRole, _ = UserRole(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := RequireAuth(jwtManager)(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !gotOK || gotID != "u1" || gotRole != "user" {
		t.Errorf("context injection = (id=%q, role=%q, ok=%v), want (u1, user, true)", gotID, gotRole, gotOK)
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	jwtManager := auth.NewJWTManager("test-secret-at-least-32-bytes-long", time.Hour)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusTeapot) })
	handler := RequireAuth(jwtManager)(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
