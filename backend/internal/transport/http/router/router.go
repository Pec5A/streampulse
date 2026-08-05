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

type Handlers struct {
	Auth  *handler.AuthHandler
	Admin *handler.AdminHandler
}

func New(h Handlers, jwtManager *auth.JWTManager) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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

	return mux
}
