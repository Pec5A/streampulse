package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/streampulse/backend/internal/infrastructure/observability"
)

func TestMetrics_RecordsRequestCountAndStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /widgets/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	handler := Metrics(mux)

	req := httptest.NewRequest(http.MethodGet, "/widgets/42", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	// The path label is the template alone; the method is its own label.
	got := testutil.ToFloat64(observability.HTTPRequestsTotal.WithLabelValues("GET", "/widgets/{id}", "201"))
	if got < 1 {
		t.Errorf("HTTPRequestsTotal for GET /widgets/{id}:201 = %v, want >= 1", got)
	}
}

func TestMetrics_CollapsesUnmatchedRoutesOntoOneLabel(t *testing.T) {
	// Guards against unbounded label cardinality: an unauthenticated scanner
	// hitting many distinct unmatched URLs must NOT mint one Prometheus
	// series per URL (a memory-exhaustion DoS). Every 404 collapses onto the
	// single "<unmatched>" label instead.
	mux := http.NewServeMux()
	handler := Metrics(mux)

	before := testutil.ToFloat64(
		observability.HTTPRequestsTotal.WithLabelValues("GET", unmatchedRoute, "404"))

	for _, p := range []string{"/x/1", "/x/2", "/x/3", "/scanner/probe"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want %d", p, rec.Code, http.StatusNotFound)
		}
	}

	// All four requests land on the single "<unmatched>" series...
	after := testutil.ToFloat64(
		observability.HTTPRequestsTotal.WithLabelValues("GET", unmatchedRoute, "404"))
	if after-before < 4 {
		t.Errorf("<unmatched> counter rose by %v, want >= 4", after-before)
	}

	// ...and never under a raw URL, which would be the unbounded-cardinality bug.
	for _, p := range []string{"/x/1", "/x/2", "/x/3", "/scanner/probe"} {
		if raw := testutil.ToFloat64(
			observability.HTTPRequestsTotal.WithLabelValues("GET", p, "404")); raw != 0 {
			t.Errorf("raw path %q leaked a series (=%v); label cardinality is unbounded", p, raw)
		}
	}
}

// The three tests below guard the transparency of statusRecorder. They exist
// because losing it does not produce an error anywhere: the listen endpoint
// keeps answering 200 and simply never delivers audio, and the WebSocket
// upgrade fails deep inside the library. The router suite went from seconds
// to a ten-minute timeout the first time streaming and this middleware met.

func TestMetrics_WrappedWriterStaysFlushable(t *testing.T) {
	// http.NewResponseController is what the live-listen handler uses; it
	// finds the real writer by following Unwrap.
	//
	// The result travels on a channel rather than a variable the test reads
	// after http.Get returns: Get returns as soon as the response headers
	// arrive, and Flush is exactly what sends them — so the handler is still
	// running at that point, and reading a plain variable would be a race
	// that fails a few runs out of a hundred.
	flushErr := make(chan error, 1)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flushErr <- http.NewResponseController(w).Flush()
	})

	srv := httptest.NewServer(Metrics(next))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/stream") //nolint:noctx // test client
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	select {
	case err := <-flushErr:
		if err != nil {
			t.Errorf("Flush() through the middleware error = %v, want nil — audio would never leave the buffer", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler never reached the flush")
	}
}

func TestMetrics_WrappedWriterStaysHijackable(t *testing.T) {
	// coder/websocket type-asserts http.Hijacker directly instead of going
	// through the response controller, so Unwrap alone is not enough.
	hijackErr := make(chan error, 1)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			hijackErr <- fmt.Errorf("wrapped writer does not implement http.Hijacker; the WebSocket publish route cannot upgrade")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			hijackErr <- err
			return
		}
		hijackErr <- nil
		_ = conn.Close()
	})

	srv := httptest.NewServer(Metrics(next))
	defer srv.Close()

	//nolint:bodyclose,noctx // the connection is hijacked and closed server-side
	_, _ = http.Get(srv.URL + "/ws")

	if err := <-hijackErr; err != nil {
		t.Error(err)
	}
}

func TestMetrics_UnwrapExposesTheRealWriter(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapped := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}

	if got := wrapped.Unwrap(); got != http.ResponseWriter(rec) {
		t.Errorf("Unwrap() = %v, want the wrapped writer", got)
	}
}

func TestMetrics_PathLabelHoldsTheTemplateWithoutTheMethod(t *testing.T) {
	// Go 1.22 patterns include the method, so using r.Pattern raw produced
	// series like method="GET", path="GET /health": the method twice, and a
	// path label that cannot be grouped cleanly in PromQL.
	observability.HTTPRequestsTotal.Reset()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/streams/{id}", func(w http.ResponseWriter, r *http.Request) {})
	handler := Metrics(mux)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/streams/abc", nil))

	got := testutil.ToFloat64(observability.HTTPRequestsTotal.WithLabelValues(
		http.MethodGet, "/api/v1/streams/{id}", "200"))
	if got != 1 {
		t.Errorf("no series for path=/api/v1/streams/{id}; the method is probably still glued to the path")
	}
	if leaked := testutil.ToFloat64(observability.HTTPRequestsTotal.WithLabelValues(
		http.MethodGet, "GET /api/v1/streams/{id}", "200")); leaked != 0 {
		t.Errorf("path label still carries the method (=%v)", leaked)
	}
}
