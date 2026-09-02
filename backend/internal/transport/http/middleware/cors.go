package middleware

import (
	"net/http"
	"slices"
	"strconv"
	"time"
)

// preflightMaxAge is how long a browser may cache the preflight answer.
const preflightMaxAge = 10 * time.Minute

// CORS answers cross-origin browser requests for the origins in allowed.
//
// Needed because the delivery pipeline publishes a `flutter build web`
// artefact: that page is served from a different origin than the API, so
// without these headers every one of its requests is blocked by the browser
// and the artefact is shipped but unusable.
//
// The allowlist is an exact match against a configured list, never a
// reflection of whatever Origin the caller sent. Reflecting is the classic
// way to end up with an API that trusts any site on the internet, and it is
// silently equivalent to "*" — except that, unlike "*", it also works with
// credentials, which is exactly what makes it dangerous.
//
// An empty allowlist means no CORS headers at all. That is the correct
// default for an API whose only client is a native app: browser access is
// opt-in, never implicit.
func CORS(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if origin != "" && slices.Contains(allowed, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				// Tells caches that the answer depends on Origin. Without it a
				// shared cache can serve one origin's headers to another.
				w.Header().Add("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				// Preflight. Answered here and not passed down: the mux would
				// return 405 for OPTIONS on routes registered for GET/POST,
				// and the browser would read that as a denial.
				if origin != "" && slices.Contains(allowed, origin) {
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
					w.Header().Set("Access-Control-Max-Age", strconv.Itoa(int(preflightMaxAge.Seconds())))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
