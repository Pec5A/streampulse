package middleware

import (
	"context"
	"net/http"

	"github.com/streampulse/backend/internal/infrastructure/auth"
)

// RequireAuthWS authenticates a WebSocket upgrade request, accepting the JWT
// either in the Authorization header or in a `token` query parameter.
//
// Why a second middleware instead of extending RequireAuth: the browser
// WebSocket API cannot set request headers, so a web broadcaster has no way
// to send `Authorization: Bearer …` on the upgrade. A query parameter is the
// standard workaround, but it is strictly worse than a header — URLs end up
// in access logs, proxy logs and browser history.
//
// So the fallback is confined to this middleware, and this middleware is
// mounted only on the WebSocket publish route. Every other authenticated
// route keeps header-only auth via RequireAuth. The mitigations are: tokens
// are short-lived (24 h, see config.JWTExpiration), the route is TLS-only in
// production, and the query parameter is never logged by our handlers.
func RequireAuthWS(jwtManager *auth.JWTManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r.Header.Get("Authorization"))
			if token == "" {
				token = r.URL.Query().Get("token")
			}
			if token == "" {
				http.Error(w, `{"error":"missing or invalid authorization"}`, http.StatusUnauthorized)
				return
			}

			claims, err := jwtManager.Validate(token)
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

// bearerToken extracts the credential from an "Bearer <token>" header,
// returning "" when the header is absent or malformed.
func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || header[:len(prefix)] != prefix {
		return ""
	}
	return header[len(prefix):]
}
