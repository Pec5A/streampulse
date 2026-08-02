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

func TestMetrics_RecordsUnmatchedRouteByRawPath(t *testing.T) {
	mux := http.NewServeMux()
	handler := Metrics(mux)

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	// No registered pattern matched, so Request.Pattern is empty and the
	// middleware falls back to the raw path — just confirm it doesn't panic
	// and still records something under the raw path label.
	got := testutil.ToFloat64(observability.HTTPRequestsTotal.WithLabelValues("GET", "/does-not-exist", "404"))
	if got < 1 {
		t.Errorf("HTTPRequestsTotal for unmatched route = %v, want >= 1", got)
	}
}
