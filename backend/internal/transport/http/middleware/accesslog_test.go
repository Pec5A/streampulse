package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/streampulse/backend/internal/infrastructure/observability"
)

// captureLogs swaps the default logger for one writing to buf, and restores it.
// AccessLog uses the package-level slog on purpose — a handler should not have
// to be handed a logger to be observable.
func captureLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	previous := slog.Default()
	// Le vrai constructeur de production, pas un handler JSON nu : c'est lui
	// qui porte l'injection de trace_id, et un test qui l'omet prouverait le
	// format sans prouver la corrélation.
	environment := "production"
	if level == slog.LevelDebug {
		environment = "development"
	}
	slog.SetDefault(observability.NewLoggerTo(&buf, environment))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return &buf
}

func serveThrough(handler http.Handler, method, pattern, target string, status int) {
	mux := http.NewServeMux()
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	})
	_ = handler
	AccessLog(mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, target, nil))
}

func decodeLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	if buf.Len() == 0 {
		t.Fatal("no log line emitted")
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("log line is not JSON (%v): %s", err, buf.String())
	}
	return got
}

func TestAccessLog_EmitsALinePerRequest(t *testing.T) {
	// The gap this middleware closes: before it, a request that succeeded
	// produced no log line at all, so there was nothing for a trace_id to
	// point at.
	buf := captureLogs(t, slog.LevelInfo)
	serveThrough(nil, http.MethodGet, "GET /api/v1/streams/{id}", "/api/v1/streams/abc", http.StatusOK)

	got := decodeLine(t, buf)
	if got["method"] != http.MethodGet {
		t.Errorf("method = %v", got["method"])
	}
	// The template, never the raw URL: one distinct route value per id would
	// make the log index unusable and unbounded.
	if got["route"] != "/api/v1/streams/{id}" {
		t.Errorf("route = %v, want the template", got["route"])
	}
	if got["status"] != float64(http.StatusOK) {
		t.Errorf("status = %v", got["status"])
	}
	if _, ok := got["duration_ms"]; !ok {
		t.Error("no duration_ms; a line without timing cannot explain a latency spike")
	}
}

func TestAccessLog_CarriesTheTraceID(t *testing.T) {
	// The whole point: the line must be joinable to the trace. Tracing sits
	// outside AccessLog in the router precisely so the span exists here.
	buf := captureLogs(t, slog.LevelInfo)
	recorder := traceRig(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health2", func(w http.ResponseWriter, r *http.Request) {})
	Tracing(AccessLog(mux)).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health2", nil))

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans))
	}

	got := decodeLine(t, buf)
	want := spans[0].SpanContext().TraceID().String()
	if got["trace_id"] != want {
		t.Errorf("trace_id = %v, want %s — the line cannot be joined to its trace", got["trace_id"], want)
	}
}

func TestAccessLog_LevelFollowsOutcome(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		status  int
		want    string
	}{
		{"success", "GET /api/v1/streams", "/api/v1/streams", http.StatusOK, "INFO"},
		// A rejected login is the server working, not failing.
		{"unauthorised", "GET /api/v1/streams", "/api/v1/streams", http.StatusUnauthorized, "INFO"},
		{"server error", "GET /api/v1/streams", "/api/v1/streams", http.StatusInternalServerError, "ERROR"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := captureLogs(t, slog.LevelDebug)
			serveThrough(nil, http.MethodGet, tc.pattern, tc.target, tc.status)

			if got := decodeLine(t, buf)["level"]; got != tc.want {
				t.Errorf("level = %v for status %d, want %s", got, tc.status, tc.want)
			}
		})
	}
}

func TestAccessLog_ProbesAreQuietAtInfo(t *testing.T) {
	// /health and /metrics are polled every few seconds by the platform and by
	// Prometheus. At info level they bury every real request.
	buf := captureLogs(t, slog.LevelInfo)
	serveThrough(nil, http.MethodGet, "GET /health", "/health", http.StatusOK)

	if buf.Len() != 0 {
		t.Errorf("probe logged at info level: %s", buf.String())
	}

	// Still available when someone is actually looking.
	buf = captureLogs(t, slog.LevelDebug)
	serveThrough(nil, http.MethodGet, "GET /health", "/health", http.StatusOK)
	if got := decodeLine(t, buf)["route"]; got != "/health" {
		t.Errorf("probe not logged at debug level either: %v", got)
	}
}

func TestAccessLog_UnmatchedRoutesCollapse(t *testing.T) {
	buf := captureLogs(t, slog.LevelInfo)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /known", func(w http.ResponseWriter, r *http.Request) {})
	AccessLog(mux).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nope/12345", nil))

	if got := decodeLine(t, buf)["route"]; got != unmatchedRoute {
		t.Errorf("route = %v, want %q — a scanner would write one value per URL", got, unmatchedRoute)
	}
}
