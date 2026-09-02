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
	Stream   *handler.StreamHandler
	Track    *handler.TrackHandler
	Admin    *handler.AdminHandler
	Playlist *handler.PlaylistHandler
}

func New(h Handlers, jwtManager *auth.JWTManager) http.Handler {
	mux := http.NewServeMux()
	authed := middleware.RequireAuth(jwtManager)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/v1/auth/register", h.Auth.Register)
	mux.HandleFunc("POST /api/v1/auth/login", h.Auth.Login)
	mux.Handle("POST /api/v1/auth/refresh", authed(http.HandlerFunc(h.Auth.Refresh)))

	// Account endpoints (ticket Y2) — the id always comes from the JWT, never
	// from the path, so there is no id to tamper with.
	mux.Handle("GET /api/v1/users/me", authed(http.HandlerFunc(h.User.Me)))
	mux.Handle("GET /api/v1/users/me/data", authed(http.HandlerFunc(h.User.ExportData)))
	mux.Handle("DELETE /api/v1/users/me", authed(http.HandlerFunc(h.User.DeleteMe)))

	// Streams — browsing and listening are public; creating, broadcasting
	// and deleting require an account (and ownership, enforced in the use
	// case, not here).
	mux.HandleFunc("GET /api/v1/streams", h.Stream.List)
	mux.HandleFunc("GET /api/v1/streams/live", h.Stream.ListLive)
	mux.HandleFunc("GET /api/v1/streams/{id}", h.Stream.Get)
	mux.HandleFunc("GET /api/v1/streams/{id}/listen", h.Stream.Listen)

	mux.Handle("POST /api/v1/streams", authed(http.HandlerFunc(h.Stream.Create)))
	mux.Handle("DELETE /api/v1/streams/{id}", authed(http.HandlerFunc(h.Stream.Delete)))
	mux.Handle("POST /api/v1/streams/{id}/publish", authed(http.HandlerFunc(h.Stream.Publish)))

	// Tracks — the catalogue and the audio itself are public (same rule as
	// listening to a live stream); uploading and deleting are not.
	mux.HandleFunc("GET /api/v1/tracks", h.Track.List)
	mux.HandleFunc("GET /api/v1/tracks/{id}", h.Track.Get)
	mux.HandleFunc("GET /api/v1/tracks/{id}/audio", h.Track.Audio)

	mux.Handle("POST /api/v1/tracks", authed(http.HandlerFunc(h.Track.Upload)))
	mux.Handle("GET /api/v1/tracks/mine", authed(http.HandlerFunc(h.Track.ListMine)))
	mux.Handle("DELETE /api/v1/tracks/{id}", authed(http.HandlerFunc(h.Track.Delete)))

	// The WebSocket publish route is the one place that also accepts the JWT
	// as a query parameter — browsers cannot set headers on an upgrade.
	// See middleware.RequireAuthWS for the trade-off.
	mux.Handle("GET /api/v1/streams/{id}/publish/ws",
		middleware.RequireAuthWS(jwtManager)(http.HandlerFunc(h.Stream.PublishWS)))

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

	return middleware.Metrics(mux)
}
