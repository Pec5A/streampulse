package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func rateRig(perMinute int) http.Handler {
	return RateLimit(perMinute, 0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// The tests below cover the finding that made this configurable: keying on
// RemoteAddr behind a load balancer gives every user the *same* key, so a
// 20/min limit becomes 20/min for the whole service — a self-inflicted denial
// of service the first time a few people sign in at once.

func TestRateLimit_BehindAProxyCountsRealClients(t *testing.T) {
	h := RateLimit(2, 1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Every request arrives from the same proxy socket; only the forwarded
	// header distinguishes the callers.
	send := func(realIP string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "10.0.0.1:443" // the load balancer
		req.Header.Set("X-Forwarded-For", realIP)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < 2; i++ {
		if got := send("203.0.113.7"); got != http.StatusOK {
			t.Fatalf("warm-up %d = %d", i, got)
		}
	}
	if got := send("203.0.113.7"); got != http.StatusTooManyRequests {
		t.Errorf("third request from the same client = %d, want 429", got)
	}
	if got := send("198.51.100.4"); got != http.StatusOK {
		t.Errorf("a different client behind the same proxy got %d — the quota is pooled", got)
	}
}

func TestRateLimit_IgnoresAForgedForwardedHeaderWhenNotBehindAProxy(t *testing.T) {
	// hops=0 is the default. A client that invents X-Forwarded-For must not
	// get a fresh quota out of it.
	h := RateLimit(2, 0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	send := func(claimed string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "203.0.113.7:54321"
		req.Header.Set("X-Forwarded-For", claimed)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < 2; i++ {
		_ = send("1.1.1." + strconv.Itoa(i))
	}
	if got := send("9.9.9.9"); got != http.StatusTooManyRequests {
		t.Errorf("a forged header bought a fresh quota (got %d, want 429)", got)
	}
}

func TestRateLimit_ForgedEntriesPrependedToARealChainAreIgnored(t *testing.T) {
	// The reason for counting from the right: an attacker can prepend entries
	// but cannot remove the one the proxy appends after theirs.
	h := RateLimit(2, 1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	send := func(forged string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "10.0.0.1:443"
		// The attacker sends "forged"; the proxy appends what it really saw.
		req.Header.Set("X-Forwarded-For", forged+", 203.0.113.7")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < 2; i++ {
		_ = send("1.1.1." + strconv.Itoa(i))
	}
	if got := send("9.9.9.9"); got != http.StatusTooManyRequests {
		t.Errorf("prepended entries changed the key (got %d, want 429)", got)
	}
}
