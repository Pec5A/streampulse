package middleware

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimit caps requests per client IP over a rolling minute.
//
// Applied to the authentication endpoints, where the absence of any limit is
// an open invitation to credential stuffing: /auth/login answers as fast as
// bcrypt allows and says nothing about how many times it has been asked.
//
// Deliberately in-memory and per-instance. A shared limiter (Redis) is the
// correct answer once the API runs on more than one machine — with N
// instances the effective limit is N times this one. Choosing the simple
// version now is a trade-off, not an oversight: it removes the trivial attack
// today without adding an external dependency the project does not otherwise
// need. The day a second instance exists, this comment is the reminder.
func RateLimit(perMinute int) func(http.Handler) http.Handler {
	if perMinute <= 0 {
		// 0 disables the limit — used by tests and by local development,
		// where locking yourself out of your own API is the likelier failure.
		return func(next http.Handler) http.Handler { return next }
	}

	l := &limiter{
		perMinute: perMinute,
		window:    time.Minute,
		hits:      make(map[string][]time.Time),
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if !l.allow(ip, time.Now()) {
				// Retry-After lets a well-behaved client back off instead of
				// hammering, and tells an honest user this is temporary.
				w.Header().Set("Retry-After", strconv.Itoa(int(l.window.Seconds())))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte(`{"error":"too many requests"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type limiter struct {
	perMinute int
	window    time.Duration

	mu   sync.Mutex
	hits map[string][]time.Time
}

func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-l.window)

	// Drop expired entries for this key, and opportunistically drop keys that
	// have gone quiet. Without the second part the map grows once per distinct
	// client IP and never shrinks — the same unbounded-growth shape as the
	// metrics label cardinality problem, reached from a different direction.
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.hits[key] = kept
	l.sweep(cutoff)

	if len(kept) >= l.perMinute {
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}

// sweep removes keys whose most recent hit fell out of the window. Called
// under the lock, on every request: the map only ever holds clients seen in
// the last minute, so its size is bounded by real traffic rather than by the
// number of IPs that ever touched the service.
func (l *limiter) sweep(cutoff time.Time) {
	for k, times := range l.hits {
		if len(times) == 0 {
			delete(l.hits, k)
			continue
		}
		if times[len(times)-1].Before(cutoff) {
			delete(l.hits, k)
		}
	}
}

// clientIP prefers the socket address and falls back to nothing else.
//
// X-Forwarded-For is NOT trusted here: any client can send it, so keying the
// limiter on it would let an attacker mint a fresh quota per request by
// varying a header. Behind a proxy that terminates TLS (Fly does), the socket
// address is the proxy's, which means the limit applies per proxy rather than
// per user — a real limitation, but a conservative one: it under-counts
// nobody. Trusting the header requires knowing the proxy's address, which is
// deployment configuration this project does not have yet.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
