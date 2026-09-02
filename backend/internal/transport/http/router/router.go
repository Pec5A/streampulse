// Package router wires handlers to routes using Go 1.22+'s pattern-based
// http.ServeMux — no external router dependency needed at this size.
package router

import (
	"encoding/json"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/streampulse/backend/internal/infrastructure/auth"
	"github.com/streampulse/backend/internal/transport/http/handler"
	"github.com/streampulse/backend/internal/transport/http/middleware"
)

type Handlers struct {
	Auth     *handler.AuthHandler
	User     *handler.UserHandler
	Admin    *handler.AdminHandler
	Playlist *handler.PlaylistHandler
}

func New(h Handlers, jwtManager *auth.JWTManager) http.Handler {
	mux := http.NewServeMux()
	requireAuth := middleware.RequireAuth(jwtManager)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/v1/auth/register", h.Auth.Register)
	mux.HandleFunc("POST /api/v1/auth/login", h.Auth.Login)
	mux.Handle("POST /api/v1/auth/refresh", requireAuth(http.HandlerFunc(h.Auth.Refresh)))

	mux.Handle("GET /api/v1/users/me", requireAuth(http.HandlerFunc(h.User.Me)))
	mux.Handle("GET /api/v1/users/me/data", requireAuth(http.HandlerFunc(h.User.ExportData)))
	mux.Handle("DELETE /api/v1/users/me", requireAuth(http.HandlerFunc(h.User.DeleteMe)))

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

	// Open scrape endpoint for local/docker-compose Prometheus. Not
	// authenticated — acceptable for now since nothing here is deployed
	// publicly yet (ticket K3); restricting /metrics at the network level
	// or behind an auth token is a hardening item for ticket S3.
	mux.Handle("GET /metrics", promhttp.Handler())

	// Tracing outermost: the span must cover the whole request, and the
	// context it injects has to reach the metrics middleware and every
	// handler below it.
	return middleware.Tracing(middleware.Metrics(mux))
}
