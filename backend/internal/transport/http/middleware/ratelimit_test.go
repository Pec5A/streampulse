package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func rateRig(perMinute int) http.Handler {
	return RateLimit(perMinute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func requestFrom(ip string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = ip + ":54321"
	return req
}

func TestRateLimit_BlocksBeyondTheQuota(t *testing.T) {
	h := rateRig(3)

	for i := 1; i <= 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, requestFrom("203.0.113.7"))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d = %d, want 200 (still inside the quota)", i, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requestFrom("203.0.113.7"))
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("request 4 = %d, want 429", rec.Code)
	}
	// Lets a well-behaved client back off instead of hammering.
	if got := rec.Header().Get("Retry-After"); got == "" {
		t.Error("429 without Retry-After")
	}
}

func TestRateLimit_CountsPerClient(t *testing.T) {
	// One noisy client must not lock everyone else out — that would turn the
	// protection into the denial of service it exists to prevent.
	h := rateRig(2)

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, requestFrom("203.0.113.7"))
		if rec.Code != http.StatusOK {
			t.Fatalf("warm-up request %d = %d", i, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, requestFrom("198.51.100.4"))
	if rec.Code != http.StatusOK {
		t.Errorf("a different client got %d; quotas are not per-IP", rec.Code)
	}
}

func TestRateLimit_ZeroDisablesTheLimit(t *testing.T) {
	h := rateRig(0)
	for i := 0; i < 50; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, requestFrom("203.0.113.7"))
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d = %d with the limit disabled", i, rec.Code)
		}
	}
}

func TestRateLimit_QuotaRefillsAfterTheWindow(t *testing.T) {
	// Driven with synthetic times rather than a sleep: a test that waits a
	// real minute never gets run.
	l := &limiter{perMinute: 2, window: time.Minute, hits: make(map[string][]time.Time)}
	base := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

	if !l.allow("ip", base) || !l.allow("ip", base.Add(time.Second)) {
		t.Fatal("the first two requests must pass")
	}
	if l.allow("ip", base.Add(2*time.Second)) {
		t.Fatal("the third request inside the window must be blocked")
	}
	if !l.allow("ip", base.Add(2*time.Minute)) {
		t.Error("the quota never refilled: a client is banned forever after one burst")
	}
}

func TestRateLimit_ForgetsQuietClients(t *testing.T) {
	// Same unbounded-growth shape as the metrics label cardinality problem,
	// reached from another direction: without a sweep the map grows by one
	// entry per distinct IP that ever touched the service, and never shrinks.
	l := &limiter{perMinute: 5, window: time.Minute, hits: make(map[string][]time.Time)}
	base := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 100; i++ {
		l.allow(string(rune('a'+i%26))+string(rune('a'+i/26)), base)
	}
	if len(l.hits) == 0 {
		t.Fatal("nothing was recorded")
	}

	l.allow("late", base.Add(5*time.Minute))
	if len(l.hits) != 1 {
		t.Errorf("map holds %d clients long after their window; want only the recent one", len(l.hits))
	}
}
