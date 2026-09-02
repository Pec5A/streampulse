package middleware

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// tracerName identifies this instrumentation in the emitted spans.
const tracerName = "github.com/streampulse/backend/internal/transport/http"

// Tracing opens a server span for every request and continues the caller's
// trace when the request carries a W3C traceparent header — that header is
// what lets a trace started in the Flutter app arrive here as the same trace
// instead of an unrelated one.
//
// The span is named after the *route template* (e.g. "GET
// /api/v1/playlists/{id}"), never the raw URL, for the same reason the metrics
// middleware labels on the template: one span name per route keeps traces
// groupable, where per-id names would produce one distinct operation per
// playlist ever opened.
//
// That name can only be set after routing, because http.ServeMux is what
// resolves the pattern. Note the request handed to the next handler is kept in
// a variable: ServeMux writes Pattern onto the request value it receives, and
// r.WithContext returns a copy — reading r.Pattern here instead of req.Pattern
// would always come back empty.
func Tracing(next http.Handler) http.Handler {
	tracer := otel.Tracer(tracerName)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		ctx, span := tracer.Start(ctx, r.Method,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodOriginal(r.Method),
				semconv.URLPath(r.URL.Path),
				semconv.UserAgentOriginal(r.UserAgent()),
			),
		)
		defer span.End()

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		req := r.WithContext(ctx)

		next.ServeHTTP(rec, req)

		// Go 1.22 patterns already carry the method ("GET /api/v1/x/{id}"),
		// which is exactly the OTel convention for a server span name. The
		// http.route attribute, however, is the path template alone.
		pattern := req.Pattern
		if pattern == "" {
			pattern = r.Method + " " + unmatchedRoute
		}
		span.SetName(pattern)
		span.SetAttributes(
			semconv.HTTPRoute(pathTemplate(pattern)),
			semconv.HTTPResponseStatusCode(rec.status),
		)

		// Only 5xx marks the span as failed. A 401 or a 404 is the server
		// working correctly; flagging those as errors makes the error rate in
		// Grafana track client behaviour instead of service health, which is
		// exactly the business/technical confusion the dashboard split exists
		// to avoid.
		if rec.status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(rec.status))
		}
	})
}

// pathTemplate drops the leading method from a ServeMux pattern. Patterns may
// be registered without one ("/health"), so a missing space is not an error.
func pathTemplate(pattern string) string {
	if _, path, found := strings.Cut(pattern, " "); found {
		return path
	}
	return pattern
}
