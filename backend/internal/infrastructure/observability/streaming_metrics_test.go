package observability_test

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/streampulse/backend/internal/infrastructure/observability"
)

func TestStreamingCollector_ReportsEveryValue(t *testing.T) {
	c := observability.NewStreamingCollector(func() observability.StreamingTotals {
		return observability.StreamingTotals{
			ActiveStreams:   3,
			ActiveListeners: 47,
			BytesPublished:  1024,
			ChunksDropped:   5,
			Evictions:       1,
		}
	})

	expected := `
# HELP streampulse_active_listeners Listeners currently attached across all live streams (business metric)
# TYPE streampulse_active_listeners gauge
streampulse_active_listeners 47
# HELP streampulse_active_streams Streams currently broadcasting (business metric)
# TYPE streampulse_active_streams gauge
streampulse_active_streams 3
# HELP streampulse_broadcast_bytes_total Total audio bytes ingested from broadcasters since process start, counted once per chunk regardless of how many listeners received it (business metric)
# TYPE streampulse_broadcast_bytes_total counter
streampulse_broadcast_bytes_total 1024
# HELP streampulse_listener_chunks_dropped_total Audio chunks dropped because a listener could not keep up (business metric)
# TYPE streampulse_listener_chunks_dropped_total counter
streampulse_listener_chunks_dropped_total 5
# HELP streampulse_listener_evictions_total Listeners disconnected for falling too far behind (business metric)
# TYPE streampulse_listener_evictions_total counter
streampulse_listener_evictions_total 1
`

	if err := testutil.CollectAndCompare(c, strings.NewReader(expected)); err != nil {
		t.Fatalf("unexpected metrics: %v", err)
	}
}

// The collector must read on every scrape, not cache the first answer:
// a gauge that never moves is worse than no gauge, because it looks healthy.
func TestStreamingCollector_ReadsOnEveryScrape(t *testing.T) {
	streams := 1
	c := observability.NewStreamingCollector(func() observability.StreamingTotals {
		return observability.StreamingTotals{ActiveStreams: streams}
	})

	if got := testutil.CollectAndCount(c); got != 5 {
		t.Fatalf("metric count = %d, want 5", got)
	}
	if got := testutil.ToFloat64(gaugeOnly(t, c, "streampulse_active_streams")); got != 1 {
		t.Fatalf("first scrape = %v, want 1", got)
	}

	streams = 9
	if got := testutil.ToFloat64(gaugeOnly(t, c, "streampulse_active_streams")); got != 9 {
		t.Fatalf("second scrape = %v, want 9 — the collector cached its first read", got)
	}
}

func TestStreamingCollector_RegistersWithoutConflict(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := observability.NewStreamingCollector(func() observability.StreamingTotals {
		return observability.StreamingTotals{}
	})
	if err := reg.Register(c); err != nil {
		t.Fatalf("Register() = %v, want nil", err)
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() = %v", err)
	}
	if len(families) != 5 {
		t.Fatalf("gathered %d families, want 5", len(families))
	}
}

// gaugeOnly re-collects c into a throwaway registry and returns a handle
// testutil.ToFloat64 accepts. The collector emits five series, so it cannot be
// passed to ToFloat64 directly — that helper requires exactly one.
func gaugeOnly(t *testing.T, c prometheus.Collector, name string) prometheus.Collector {
	t.Helper()

	reg := prometheus.NewPedanticRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("Register() = %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() = %v", err)
	}

	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		g := prometheus.NewGauge(prometheus.GaugeOpts{Name: "probe", Help: "probe"})
		m := f.GetMetric()[0]
		if m.GetGauge() != nil {
			g.Set(m.GetGauge().GetValue())
		} else {
			g.Set(m.GetCounter().GetValue())
		}
		return g
	}
	t.Fatalf("metric %q not collected", name)
	return nil
}
