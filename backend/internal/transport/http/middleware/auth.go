// Package middleware provides stdlib net/http middleware (no framework):
// this is a small enough API that Go 1.22+'s pattern-based ServeMux covers
// routing needs without adding a router dependency.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/streampulse/backend/internal/infrastructure/auth"
)

type contextKey string

const (
	userIDKey contextKey = "user_id"
	roleKey   contextKey = "user_role"
)

// RequireAuth validates the Bearer JWT and injects user_id/role into the
// request context for downstream handlers.
func RequireAuth(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, `{"error":"missing or invalid authorization header"}`, http.StatusUnauthorized)
				return
			}

			claims, err := jwtManager.Validate(parts[1])
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			ctx = context.WithValue(ctx, roleKey, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserID reads the authenticated user id set by RequireAuth.
func UserID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey).(string)
	return v, ok
}

// UserRole reads the authenticated user's role set by RequireAuth.
func UserRole(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(roleKey).(string)
	return v, ok
}
