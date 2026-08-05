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

// TestRouter_PlaylistRoutesRegisteredAndProtected exercises the playlist route
// block: the routes exist and every one sits behind RequireAuth (a request
// without a token is rejected before the handler/use case is reached, so a nil
// use case is safe here).
func TestRouter_PlaylistRoutesRegisteredAndProtected(t *testing.T) {
	jwt := auth.NewJWTManager("test-secret-at-least-32-bytes-long!!", time.Hour)
	srv := router.New(router.Handlers{Playlist: handler.NewPlaylistHandler(nil)}, jwt)

	cases := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/playlists"},
		{http.MethodGet, "/api/v1/playlists"},
		{http.MethodGet, "/api/v1/playlists/abc"},
		{http.MethodPut, "/api/v1/playlists/abc/tracks/order"},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token -> %d, want 401", tc.method, tc.path, w.Code)
		}
	}
}
