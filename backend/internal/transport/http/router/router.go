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
	Auth *handler.AuthHandler
	User *handler.UserHandler
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

	return mux
}
