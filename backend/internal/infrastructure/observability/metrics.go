// Package observability holds Prometheus metrics, split deliberately into
// business metrics (what a product owner cares about: logins, signups) and
// technical metrics (what an SRE cares about: latency, error rates) — see
// docs/adr/0002-observability.md for why that split matters on the
// Grafana dashboard.
package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// --- Business metrics ---

	AuthLoginsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "streampulse_auth_logins_total",
			Help: "Total login attempts, labelled by outcome (business metric)",
		},
		[]string{"result"}, // success | invalid_credentials | error
	)

	AuthRegistrationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "streampulse_auth_registrations_total",
			Help: "Total registration attempts, labelled by outcome (business metric)",
		},
		[]string{"result"}, // success | conflict | error
	)

	// Time a listener waits between attaching to a live stream and hearing
	// the first byte of audio. This is the quality metric the product is
	// actually judged on — a listener does not experience "the API answered
	// in 3ms", they experience how long they stare at a silent player.
	//
	// Deliberately not derived from the HTTP duration histogram: a listen
	// request lasts as long as the broadcast, so its duration measures the
	// length of the session, not the wait before it starts.
	ListenerTimeToFirstChunk = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name: "streampulse_listener_time_to_first_chunk_seconds",
			Help: "Time between a listener attaching and receiving its first audio chunk (business metric)",
			// The default buckets top out at 10s and crowd the sub-10ms range,
			// which is the wrong resolution here: nothing useful happens below
			// 50ms and a listener waiting a minute is a case worth seeing.
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60},
		},
	)

	// --- Technical metrics ---

	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "streampulse_http_requests_total",
			Help: "Total HTTP requests, labelled by route and status (technical metric)",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "streampulse_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds (technical metric)",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

// Prometheus counter vectors only expose a label combination once it has been
// incremented, so a freshly started process publishes none of these: the
// dashboard panel reads "No data" until the first login, and a ratio alert
// has nothing to evaluate. Declaring the outcomes up front makes them exist
// at zero from the first scrape, which is also the only way a *drop* to zero
// is distinguishable from a process that has simply never seen traffic.
func init() {
	for _, result := range []string{"success", "invalid_credentials", "error"} {
		AuthLoginsTotal.WithLabelValues(result)
	}
	for _, result := range []string{"success", "conflict", "error"} {
		AuthRegistrationsTotal.WithLabelValues(result)
	}
}
