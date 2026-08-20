package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

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

	got := testutil.ToFloat64(observability.HTTPRequestsTotal.WithLabelValues("GET", "GET /widgets/{id}", "201"))
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
