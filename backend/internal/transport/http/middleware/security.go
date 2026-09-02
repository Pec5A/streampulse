package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// SecurityHeaders sets the response headers that cost nothing and close whole
// classes of browser-side attack.
//
// They matter here because the delivery pipeline ships a web build: the API's
// answers are read by a browser, so its headers are part of that page's
// security, not just decoration on a JSON body.
//
// HSTS is only sent outside development. Sent from a plain-HTTP dev server it
// would pin localhost to HTTPS in the developer's browser — for a year, across
// every project using that port.
func SecurityHeaders(environment string) func(http.Handler) http.Handler {
	production := environment != "development"

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			// Stops a browser from second-guessing Content-Type — the reason
			// an uploaded file served as audio/mpeg cannot be re-read as HTML.
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			// The API returns JSON, never markup: forbidding every source is
			// accurate rather than merely restrictive.
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

			if production {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireMetricsToken guards the Prometheus scrape endpoint with a bearer
// token.
//
// /metrics is not an innocuous endpoint: it publishes the full route table,
// request volumes per route, latency distributions and the process's memory
// profile. That is a map of the application handed to whoever asks, and the
// brief names it explicitly among the endpoints to secure.
//
// An empty token disables the guard, which config.Load permits only in
// development — outside it, startup fails rather than serving this openly.
func RequireMetricsToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if token == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presented := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			// Constant-time: a byte-by-byte comparison leaks the token one
			// character at a time to anyone willing to measure.
			if subtle.ConstantTimeCompare([]byte(presented), []byte(token)) != 1 {
				// 404 rather than 401: an unauthenticated scanner learns
				// nothing, not even that the endpoint exists.
				http.NotFound(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
