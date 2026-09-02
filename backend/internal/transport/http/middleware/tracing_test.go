package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// traceRig installs a recording provider globally (Tracing reads the global,
// as production code does) and restores the previous one afterwards.
func traceRig(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	return recorder
}

// muxWith routes pattern to h, so r.Pattern is populated the way it is in
// production. Calling the middleware on a bare handler would leave Pattern
// empty and quietly test nothing.
func muxWith(pattern string, h http.HandlerFunc) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(pattern, h)
	return Tracing(mux)
}

func attrValue(span sdktrace.ReadOnlySpan, key attribute.Key) attribute.Value {
	for _, a := range span.Attributes() {
		if a.Key == key {
			return a.Value
		}
	}
	return attribute.Value{}
}

func TestTracing_NamesTheSpanAfterTheRouteTemplate(t *testing.T) {
	recorder := traceRig(t)

	handler := muxWith("GET /api/v1/playlists/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/playlists/abc-123", nil))

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	// The raw id must not appear in the name: one span name per playlist ever
	// opened makes traces impossible to group.
	if got, want := spans[0].Name(), "GET /api/v1/playlists/{id}"; got != want {
		t.Errorf("span name = %q, want %q", got, want)
	}
	if got := attrValue(spans[0], "http.route").AsString(); got != "/api/v1/playlists/{id}" {
		t.Errorf("http.route = %q, want the template", got)
	}
	if got := attrValue(spans[0], "http.response.status_code").AsInt64(); got != http.StatusOK {
		t.Errorf("status code attribute = %d, want 200", got)
	}
	if spans[0].SpanKind() != trace.SpanKindServer {
		t.Errorf("span kind = %v, want server", spans[0].SpanKind())
	}
}

func TestTracing_ContinuesAnIncomingTrace(t *testing.T) {
	recorder := traceRig(t)

	handler := muxWith("GET /health", func(w http.ResponseWriter, r *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	const upstreamTrace = "4bf92f3577b34da6a3ce929d0e0e4736"
	req.Header.Set("traceparent", "00-"+upstreamTrace+"-00f067aa0ba902b7-01")

	handler.ServeHTTP(httptest.NewRecorder(), req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	// This is the whole point of distributed tracing: the span the mobile app
	// started and this one must belong to the same trace.
	if got := spans[0].SpanContext().TraceID().String(); got != upstreamTrace {
		t.Errorf("trace id = %s, want the caller's %s — the trace was broken here", got, upstreamTrace)
	}
	if !spans[0].Parent().IsValid() {
		t.Error("span has no parent; the incoming context was dropped")
	}
}

func TestTracing_MarksOnlyServerErrorsAsFailed(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		wantError bool
	}{
		{"ok", http.StatusOK, false},
		// A rejected login is the server working, not the server failing.
		{"unauthorised", http.StatusUnauthorized, false},
		{"not found", http.StatusNotFound, false},
		{"server error", http.StatusInternalServerError, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := traceRig(t)

			handler := muxWith("GET /probe", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
			})
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/probe", nil))

			spans := recorder.Ended()
			if len(spans) != 1 {
				t.Fatalf("got %d spans, want 1", len(spans))
			}
			gotError := spans[0].Status().Code == codes.Error
			if gotError != tc.wantError {
				t.Errorf("span error = %v for status %d, want %v", gotError, tc.status, tc.wantError)
			}
		})
	}
}

func TestTracing_CollapsesUnmatchedRoutes(t *testing.T) {
	// Same reason as the metrics label: a scanner walking /x/1, /x/2, … must
	// not mint a new span name per URL.
	recorder := traceRig(t)

	handler := muxWith("GET /health", func(w http.ResponseWriter, r *http.Request) {})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nope/12345", nil))

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}
	if got, want := spans[0].Name(), "GET "+unmatchedRoute; got != want {
		t.Errorf("span name = %q, want %q", got, want)
	}
}
