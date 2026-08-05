package middleware

import "net/http"

// RequireAdmin rejects callers whose role isn't "admin". It must be chained
// after RequireAuth, which puts the authenticated user's role into the request
// context.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if role, ok := UserRole(r.Context()); !ok || role != "admin" {
			http.Error(w, `{"error":"admin access required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
