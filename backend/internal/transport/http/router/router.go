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
	Admin    *handler.AdminHandler
	Playlist *handler.PlaylistHandler
}

// Options carries the deployment-dependent hardening settings. Variadic on
// New so the many test call sites that do not care keep compiling — and so
// that the zero value is the safe one: no CORS, no metrics exposure guard
// needed (development), no rate limit.
type Options struct {
	Environment    string
	AllowedOrigins []string
	MetricsToken   string
	AuthRateLimit  int
	// TrustedProxyHops decides where the real client address is read from.
	// Getting it wrong pools every user into one quota — see middleware.clientIP.
	TrustedProxyHops int
}

func New(h Handlers, jwtManager *auth.JWTManager, opts ...Options) http.Handler {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}

	mux := http.NewServeMux()
	authed := middleware.RequireAuth(jwtManager)
	// Credential stuffing is the attack this closes: bcrypt makes each attempt
	// slow, nothing made them few.
	authLimit := middleware.RateLimit(o.AuthRateLimit, o.TrustedProxyHops)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.Handle("POST /api/v1/auth/register", authLimit(http.HandlerFunc(h.Auth.Register)))
	mux.Handle("POST /api/v1/auth/login", authLimit(http.HandlerFunc(h.Auth.Login)))
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

	// The scrape endpoint publishes the route table, per-route volumes,
	// latency distributions and the process memory profile — a map of the
	// application. Guarded by a bearer token, which config.Load makes
	// mandatory outside development.
	mux.Handle("GET /metrics", middleware.RequireMetricsToken(o.MetricsToken)(promhttp.Handler()))

	// Order matters, outermost first:
	//   Tracing          the span must cover everything, including rejections
	//   SecurityHeaders  set before any handler can start writing a body
	//   Metrics          so a preflight or a 429 still shows up in the counters
	//   CORS             answers preflights itself; the mux would 405 them
	return middleware.Tracing(
		middleware.SecurityHeaders(o.Environment)(
			middleware.Metrics(
				middleware.CORS(o.AllowedOrigins)(mux),
			),
		),
	)
}
