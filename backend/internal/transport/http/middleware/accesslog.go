package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// AccessLog emits one structured line per request.
//
// Without it the correlation this project builds is real but empty: the trace
// handler stamps trace_id on every line emitted inside a span, and there were
// three such lines in the whole backend — all on error paths in the streaming
// handler. A request that succeeded produced nothing, so "open the trace from a
// latency spike, paste the trace_id into Loki, get that request's lines" gave
// back nothing to read. Caught in review by @monkeyDkz: the wiring was correct
// and had no emitters.
//
// slog.InfoContext, not slog.Info: the context is what carries the span, and a
// line logged without it can never be joined to a trace.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		req := r

		next.ServeHTTP(rec, req)

		route := pathTemplate(req.Pattern)
		if route == "" {
			// Same bounded label as the metrics middleware: an unauthenticated
			// scanner must not be able to write one distinct route value per
			// URL it invents into the log index.
			route = unmatchedRoute
		}

		attrs := []any{
			"method", r.Method,
			"route", route,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		}

		switch {
		case rec.status >= http.StatusInternalServerError:
			// Only 5xx is the service failing. A 401 or a 404 is it working.
			slog.ErrorContext(req.Context(), "http request", attrs...)
		case isProbe(route):
			// Health and metrics are polled every few seconds by the platform
			// and by Prometheus. At info level they drown every real request;
			// at debug they stay available when someone is actually looking.
			slog.DebugContext(req.Context(), "http request", attrs...)
		default:
			slog.InfoContext(req.Context(), "http request", attrs...)
		}
	})
}

func isProbe(route string) bool {
	return route == "/health" || route == "/metrics"
}
