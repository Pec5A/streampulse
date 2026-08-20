package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/streampulse/backend/internal/infrastructure/observability"
)

// statusRecorder captures the status code a handler writes, since
// http.ResponseWriter doesn't expose it after the fact.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// unmatchedRoute is the fixed path label used when no route pattern matched
// (a 404). It must be a constant, never the raw URL: an unauthenticated
// scanner hitting /x/1, /x/2, … would otherwise mint one Prometheus time
// series per distinct URL (and the latency histogram multiplies that by its
// buckets), an unbounded-cardinality memory-exhaustion DoS. Collapsing every
// unmatched request onto one label keeps cardinality bounded by the number
// of real routes.
const unmatchedRoute = "<unmatched>"

// Metrics records technical metrics (request count + latency) for every
// request. r.Pattern (Go 1.22+) gives the route template (e.g.
// "/api/v1/streams/{id}"), not the raw URL — so metrics aren't fragmented
// by every distinct id value.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		path := r.Pattern
		if path == "" {
			path = unmatchedRoute
		}
		observability.HTTPRequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(rec.status)).Inc()
		observability.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(time.Since(start).Seconds())
	})
}
