package middleware

import (
	"bufio"
	"fmt"
	"net"
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

// Unwrap, Flush and Hijack keep the wrapper transparent to everything the
// handlers below actually need from a ResponseWriter.
//
// Without them this middleware silently breaks the two features that matter
// most here, and breaks them in the worst way — no error, no log, just a hang:
//
//   - Live listening calls http.NewResponseController(w).Flush(). The
//     controller finds the real writer by following Unwrap; with no Unwrap it
//     returns ErrNotSupported, the handler discards that error, and the audio
//     chunks sit in the buffer forever. Listeners connect, get a 200, and hear
//     nothing.
//   - The WebSocket publish route needs Hijack. coder/websocket type-asserts
//     http.Hijacker directly rather than going through the controller, so
//     Unwrap alone does not cover it — the upgrade has to be forwarded by
//     hand.
//
// Found by the merge that first put streaming and this middleware in the same
// binary: the router suite went from seconds to a 10-minute timeout.
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
	}
	return hj.Hijack()
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

		// pathTemplate strips the method that Go 1.22 patterns carry
		// ("GET /api/v1/streams/{id}"). Keeping it would repeat the method
		// inside the path label — series read `method="GET", path="GET
		// /health"` — which makes every `sum by (path)` awkward and the two
		// labels redundant.
		path := pathTemplate(r.Pattern)
		if path == "" {
			path = unmatchedRoute
		}
		observability.HTTPRequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(rec.status)).Inc()
		observability.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(time.Since(start).Seconds())
	})
}
