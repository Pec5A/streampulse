package router_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/streampulse/backend/internal/infrastructure/auth"
	"github.com/streampulse/backend/internal/transport/http/handler"
	"github.com/streampulse/backend/internal/transport/http/router"
)

// TestRouter_AdminRoutesRequireAuthAndAdmin exercises the admin route block:
// the routes exist and sit behind RequireAuth + RequireAdmin, both of which
// reject before the (nil) use case is ever reached.
func TestRouter_AdminRoutesRequireAuthAndAdmin(t *testing.T) {
	jwt := auth.NewJWTManager("test-secret-at-least-32-bytes-long!!", time.Hour)
	srv := router.New(router.Handlers{Admin: handler.NewAdminHandler(nil)}, jwt)

	for _, path := range []string{"/api/v1/admin/stats", "/api/v1/admin/users"} {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without token -> %d, want 401", path, w.Code)
		}
	}

	userTok, err := jwt.Generate("u", "user")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer "+userTok)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-admin token -> %d, want 403", w.Code)
	}
}
