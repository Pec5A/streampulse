package observability

import "github.com/prometheus/client_golang/prometheus"

// StreamingTotals is a point-in-time view of live broadcasting activity.
//
// It is declared here rather than imported from the streaming package so that
// observability keeps zero knowledge of how broadcasting works: the wiring
// site adapts the registry to this shape, and the collector below stays
// testable with a one-line fake.
type StreamingTotals struct {
	// ActiveStreams is how many hubs are currently open.
	ActiveStreams int
	// ActiveListeners is the total number of attached listeners across every
	// live stream.
	ActiveListeners int
	// SessionsStarted, BytesPublished, ChunksDropped and Evictions are
	// process-wide and monotonic: they include streams that have already
	// ended.
	//
	// SessionsStarted is what a gauge structurally cannot answer: a broadcast
	// that starts and ends between two scrapes never appears in
	// ActiveStreams, so "how many lives happened yesterday" needs its own
	// counter.
	SessionsStarted int64
	BytesPublished  int64
	ChunksDropped   int64
	Evictions       int64
}

// StreamingCollector reports live broadcasting state to Prometheus.
//
// It is a custom collector rather than a set of package-level gauges kept up
// to date by the hub, for two reasons:
//
//   - Correctness. A gauge maintained by hand needs an increment on every
//     subscribe and a matching decrement on every one of the four ways a
//     listener can leave (unsubscribe, eviction, hub close, process shutdown).
//     Miss one and the gauge drifts away from reality for the lifetime of the
//     process, silently. Reading the live count at scrape time cannot drift.
//
//   - Isolation. Publish is the hot path — it runs once per audio chunk, per
//     stream. Keeping Prometheus out of it means the metric cannot slow down
//     the thing it measures.
//
// Labels are deliberately absent. Labelling by stream id would create one time
// series per broadcast ever started, which is unbounded cardinality — the
// classic way to take down a Prometheus. Per-stream detail belongs in traces,
// where high-cardinality identifiers are free.
type StreamingCollector struct {
	read func() StreamingTotals

	activeStreams   *prometheus.Desc
	activeListeners *prometheus.Desc
	sessionsTotal   *prometheus.Desc
	bytesTotal      *prometheus.Desc
	dropsTotal      *prometheus.Desc
	evictionsTotal  *prometheus.Desc
}

// NewStreamingCollector builds a collector that calls read on every scrape.
//
// read must be safe for concurrent use and must not block: Prometheus scrapes
// on its own goroutine and a slow read stalls the whole /metrics response.
func NewStreamingCollector(read func() StreamingTotals) *StreamingCollector {
	return &StreamingCollector{
		read: read,
		activeStreams: prometheus.NewDesc(
			"streampulse_active_streams",
			"Streams currently broadcasting (business metric)",
			nil, nil,
		),
		activeListeners: prometheus.NewDesc(
			"streampulse_active_listeners",
			"Listeners currently attached across all live streams (business metric)",
			nil, nil,
		),
		sessionsTotal: prometheus.NewDesc(
			"streampulse_broadcast_sessions_started_total",
			"Total live broadcast sessions opened since process start (business metric)",
			nil, nil,
		),
		bytesTotal: prometheus.NewDesc(
			"streampulse_broadcast_bytes_total",
			"Total audio bytes ingested from broadcasters since process start, counted once per chunk regardless of how many listeners received it (business metric)",
			nil, nil,
		),
		dropsTotal: prometheus.NewDesc(
			"streampulse_listener_chunks_dropped_total",
			"Audio chunks dropped because a listener could not keep up (business metric)",
			nil, nil,
		),
		evictionsTotal: prometheus.NewDesc(
			"streampulse_listener_evictions_total",
			"Listeners disconnected for falling too far behind (business metric)",
			nil, nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (c *StreamingCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.activeStreams
	ch <- c.activeListeners
	ch <- c.sessionsTotal
	ch <- c.bytesTotal
	ch <- c.dropsTotal
	ch <- c.evictionsTotal
}

// Collect implements prometheus.Collector.
func (c *StreamingCollector) Collect(ch chan<- prometheus.Metric) {
	t := c.read()

	ch <- prometheus.MustNewConstMetric(c.activeStreams, prometheus.GaugeValue, float64(t.ActiveStreams))
	ch <- prometheus.MustNewConstMetric(c.activeListeners, prometheus.GaugeValue, float64(t.ActiveListeners))
	ch <- prometheus.MustNewConstMetric(c.sessionsTotal, prometheus.CounterValue, float64(t.SessionsStarted))
	ch <- prometheus.MustNewConstMetric(c.bytesTotal, prometheus.CounterValue, float64(t.BytesPublished))
	ch <- prometheus.MustNewConstMetric(c.dropsTotal, prometheus.CounterValue, float64(t.ChunksDropped))
	ch <- prometheus.MustNewConstMetric(c.evictionsTotal, prometheus.CounterValue, float64(t.Evictions))
}
