package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func securityRig(environment string) http.Handler {
	return SecurityHeaders(environment)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func TestSecurityHeaders_AlwaysSet(t *testing.T) {
	rec := httptest.NewRecorder()
	securityRig("production").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}
	for header, value := range want {
		if got := rec.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
	if got := rec.Header().Get("Content-Security-Policy"); got == "" {
		t.Error("no CSP; the API returns JSON so forbidding every source is accurate, not merely strict")
	}
}

func TestSecurityHeaders_HSTSOnlyOutsideDevelopment(t *testing.T) {
	// Sent from a plain-HTTP dev server, HSTS pins localhost to HTTPS in the
	// developer's browser for a year — across every project on that port.
	tests := []struct {
		environment string
		wantHSTS    bool
	}{
		{"development", false},
		{"staging", true},
		{"production", true},
	}

	for _, tc := range tests {
		t.Run(tc.environment, func(t *testing.T) {
			rec := httptest.NewRecorder()
			securityRig(tc.environment).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

			got := rec.Header().Get("Strict-Transport-Security") != ""
			if got != tc.wantHSTS {
				t.Errorf("HSTS present = %v in %s, want %v", got, tc.environment, tc.wantHSTS)
			}
		})
	}
}

func TestRequireMetricsToken_RejectsWithoutLeakingExistence(t *testing.T) {
	h := RequireMetricsToken("s3cret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("go_goroutines 19"))
	}))

	tests := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"wrong token", "Bearer wrong"},
		{"token without the scheme", "s3cret-but-unprefixed"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			// 404 rather than 401: a scanner learns nothing, not even that
			// the endpoint exists.
			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404", rec.Code)
			}
			if rec.Body.Len() > 0 && rec.Body.String() != "404 page not found\n" {
				t.Errorf("body leaked metrics: %q", rec.Body.String())
			}
		})
	}
}

func TestRequireMetricsToken_AcceptsTheRightToken(t *testing.T) {
	h := RequireMetricsToken("s3cret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("go_goroutines 19"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer s3cret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — Prometheus could not scrape", rec.Code)
	}
	if rec.Body.String() != "go_goroutines 19" {
		t.Errorf("body = %q", rec.Body.String())
	}
}

func TestRequireMetricsToken_EmptyTokenLeavesTheEndpointOpen(t *testing.T) {
	// Only reachable in development: config.Load refuses to start elsewhere
	// without a token.
	h := RequireMetricsToken("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 with no token configured", rec.Code)
	}
}
