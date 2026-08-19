package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireAdmin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	tests := []struct {
		name string
		ctx  context.Context
		want int
	}{
		{"no role in context", context.Background(), http.StatusForbidden},
		{"non-admin role", context.WithValue(context.Background(), roleKey, "user"), http.StatusForbidden},
		{"admin role", context.WithValue(context.Background(), roleKey, "admin"), http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(tc.ctx)
			w := httptest.NewRecorder()
			RequireAdmin(next).ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("code = %d, want %d", w.Code, tc.want)
			}
		})
	}
}
