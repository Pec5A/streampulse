// Package router wires handlers to routes using Go 1.22+'s pattern-based
// http.ServeMux — no external router dependency needed at this size.
package router

import (
	"encoding/json"
	"net/http"

	"github.com/streampulse/backend/internal/infrastructure/auth"
	"github.com/streampulse/backend/internal/transport/http/handler"
	"github.com/streampulse/backend/internal/transport/http/middleware"
)

// BuildInfo identifies the running binary. Reported by /health so that
// "which version is actually deployed" is answerable from outside the
// cluster — a rollback decision cannot wait on somebody SSHing in to read
// an image tag.
type BuildInfo struct {
	Version string
	Commit  string
}

type Handlers struct {
	Build BuildInfo

	Auth     *handler.AuthHandler
	Admin    *handler.AdminHandler
	Playlist *handler.PlaylistHandler
}

func New(h Handlers, jwtManager *auth.JWTManager) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := map[string]string{"status": "ok"}
		// Omitted rather than reported empty when the binary was not built
		// through the release pipeline: an empty version string in a probe
		// response is worse than no field, it looks like a deployed unknown.
		if h.Build.Version != "" {
			body["version"] = h.Build.Version
		}
		if h.Build.Commit != "" {
			body["commit"] = h.Build.Commit
		}
		_ = json.NewEncoder(w).Encode(body)
	})

	mux.HandleFunc("POST /api/v1/auth/register", h.Auth.Register)
	mux.HandleFunc("POST /api/v1/auth/login", h.Auth.Login)
	mux.Handle("POST /api/v1/auth/refresh", middleware.RequireAuth(jwtManager)(http.HandlerFunc(h.Auth.Refresh)))

	// Admin area (ticket S2) — every route requires a valid JWT AND the admin
	// role (RequireAuth then RequireAdmin).
	if h.Admin != nil {
		admin := func(fn http.HandlerFunc) http.Handler {
			return middleware.RequireAuth(jwtManager)(middleware.RequireAdmin(http.HandlerFunc(fn)))
		}
		mux.Handle("GET /api/v1/admin/stats", admin(h.Admin.Stats))
		mux.Handle("GET /api/v1/admin/users", admin(h.Admin.ListUsers))
		mux.Handle("PATCH /api/v1/admin/users/{id}/role", admin(h.Admin.UpdateRole))
	}

	// Playlists (ticket S1) — every route requires a valid JWT; the caller is
	// the resource owner.
	if h.Playlist != nil {
		protected := func(fn http.HandlerFunc) http.Handler {
			return middleware.RequireAuth(jwtManager)(fn)
		}
		mux.Handle("POST /api/v1/playlists", protected(h.Playlist.Create))
		mux.Handle("GET /api/v1/playlists", protected(h.Playlist.List))
		mux.Handle("GET /api/v1/playlists/{id}", protected(h.Playlist.Get))
		mux.Handle("PATCH /api/v1/playlists/{id}", protected(h.Playlist.Update))
		mux.Handle("DELETE /api/v1/playlists/{id}", protected(h.Playlist.Delete))
		mux.Handle("POST /api/v1/playlists/{id}/tracks", protected(h.Playlist.AddTrack))
		mux.Handle("DELETE /api/v1/playlists/{id}/tracks/{trackID}", protected(h.Playlist.RemoveTrack))
		mux.Handle("PUT /api/v1/playlists/{id}/tracks/order", protected(h.Playlist.Reorder))
	}

	return mux
}
