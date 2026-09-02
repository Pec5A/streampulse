package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func corsRig(allowed ...string) http.Handler {
	return CORS(allowed)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func TestCORS_AllowsAConfiguredOrigin(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/streams", nil)
	req.Header.Set("Origin", "https://streampulse.example")

	corsRig("https://streampulse.example").ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://streampulse.example" {
		t.Errorf("Allow-Origin = %q, want the configured origin", got)
	}
	// Without Vary, a shared cache can hand one origin's response to another.
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}
}

func TestCORS_RejectsAnUnlistedOrigin(t *testing.T) {
	// The failure mode this guards: reflecting whatever Origin arrives, which
	// silently trusts every site on the internet.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/streams", nil)
	req.Header.Set("Origin", "https://evil.example")

	corsRig("https://streampulse.example").ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty — the origin was reflected", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d; a disallowed origin must still get its response, the browser is what blocks it", rec.Code)
	}
}

func TestCORS_EmptyAllowlistSendsNothing(t *testing.T) {
	// Default posture: browser access is opt-in, never implicit.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/streams", nil)
	req.Header.Set("Origin", "https://streampulse.example")

	corsRig().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q with an empty allowlist, want empty", got)
	}
}

func TestCORS_AnswersPreflightItself(t *testing.T) {
	// The mux answers 405 for OPTIONS on a route registered as POST, and a
	// browser reads that as a denial — so the preflight must stop here.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "https://streampulse.example")
	req.Header.Set("Access-Control-Request-Method", "POST")

	reached := false
	CORS([]string{"https://streampulse.example"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	})).ServeHTTP(rec, req)

	if reached {
		t.Error("preflight reached the handler instead of being answered by the middleware")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("preflight answer carries no Allow-Headers; Authorization would be rejected")
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got == "" {
		t.Error("no Max-Age: every request would be preceded by a fresh preflight")
	}
}

func TestCORS_PreflightFromAnUnlistedOriginGetsNoPermission(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", "POST")

	corsRig("https://streampulse.example").ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "" {
		t.Errorf("Allow-Methods = %q for an unlisted origin, want empty", got)
	}
}
