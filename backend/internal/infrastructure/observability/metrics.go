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
