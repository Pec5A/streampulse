package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/streampulse/backend/internal/application/usecase"
	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/infrastructure/auth"
	"github.com/streampulse/backend/internal/transport/http/middleware"
)

// newTestJWTManager uses the same fixed test secret as auth_handler_test.go
// so tokens generated here are valid for middleware.RequireAuth in tests.
func newTestJWTManager() *auth.JWTManager {
	return auth.NewJWTManager("test-secret-at-least-32-bytes-long", time.Hour)
}

func newTestUserHandler() (*UserHandler, *fakeUserRepo) {
	repo := newFakeUserRepo()
	return NewUserHandler(usecase.NewUserUseCase(repo)), repo
}

func seedUser(t *testing.T, repo *fakeUserRepo, email, username string) string {
	t.Helper()
	u := &entity.User{Email: email, Username: username, Password: "hash", Role: entity.RoleUser}
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u.ID
}

func TestUserHandler_Me_RequiresAuth(t *testing.T) {
	h, _ := newTestUserHandler()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.Me(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestUserHandler_Me_ViaRealMiddleware(t *testing.T) {
	jwtManager := newTestJWTManager()
	h, repo := newTestUserHandler()
	id := seedUser(t, repo, "me@b.com", "meuser")
	token, err := jwtManager.Generate(id, "user")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	protected := middleware.RequireAuth(jwtManager)(http.HandlerFunc(h.Me))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["username"] != "meuser" {
		t.Errorf("username = %v, want meuser", body["username"])
	}
}

func TestUserHandler_ExportData_SanitizesPassword(t *testing.T) {
	jwtManager := newTestJWTManager()
	h, repo := newTestUserHandler()
	id := seedUser(t, repo, "export@b.com", "exporter")
	token, err := jwtManager.Generate(id, "user")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	protected := middleware.RequireAuth(jwtManager)(http.HandlerFunc(h.ExportData))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if body := rec.Body.String(); jsonHasKey(body, "password") || jsonHasKey(body, "Password") {
		t.Errorf("export leaked a password field: %s", body)
	}
}

func TestUserHandler_DeleteMe(t *testing.T) {
	jwtManager := newTestJWTManager()
	h, repo := newTestUserHandler()
	id := seedUser(t, repo, "del@b.com", "deleter")
	token, err := jwtManager.Generate(id, "user")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	protected := middleware.RequireAuth(jwtManager)(http.HandlerFunc(h.DeleteMe))
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if _, err := repo.FindByID(context.Background(), id); err == nil {
		t.Fatal("user still present after DeleteMe")
	}
}

// TestUserHandler_DeleteMe_Retry exercises the exact scenario flagged in
// review: the caller's JWT stays valid for its full TTL after the account
// row is gone, so a retried DELETE (double tap, network retry) must not
// surface as a 500 — it must stay a clean 204, both times.
func TestUserHandler_DeleteMe_Retry(t *testing.T) {
	jwtManager := newTestJWTManager()
	h, repo := newTestUserHandler()
	id := seedUser(t, repo, "retry@b.com", "retryer")
	token, err := jwtManager.Generate(id, "user")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	protected := middleware.RequireAuth(jwtManager)(http.HandlerFunc(h.DeleteMe))
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodDelete, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("call #%d: status = %d, want %d, body=%s", i+1, rec.Code, http.StatusNoContent, rec.Body.String())
		}
	}
}

func jsonHasKey(body, key string) bool {
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}
