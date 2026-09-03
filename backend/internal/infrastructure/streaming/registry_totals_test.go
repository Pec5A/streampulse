package streaming

import (
	"context"
	"testing"
)

func TestTotals_EmptyRegistry(t *testing.T) {
	r := NewRegistry()

	got := r.Totals()
	if got != (Totals{}) {
		t.Fatalf("Totals() = %+v, want zero value", got)
	}
}

func TestTotals_CountsLiveStreamsAndListeners(t *testing.T) {
	r := NewRegistry()
	ctx := context.Background()

	a := r.Open(ctx, "stream-a")
	r.Open(ctx, "stream-b")

	for range 3 {
		if _, _, _, err := a.Subscribe(); err != nil {
			t.Fatalf("Subscribe() = %v", err)
		}
	}

	got := r.Totals()
	if got.ActiveStreams != 2 {
		t.Errorf("ActiveStreams = %d, want 2", got.ActiveStreams)
	}
	if got.ActiveListeners != 3 {
		t.Errorf("ActiveListeners = %d, want 3", got.ActiveListeners)
	}
}

func TestTotals_ListenersDropWhenTheyLeave(t *testing.T) {
	r := NewRegistry()
	h := r.Open(context.Background(), "s")

	_, _, unsubscribe, err := h.Subscribe()
	if err != nil {
		t.Fatalf("Subscribe() = %v", err)
	}
	if got := r.Totals().ActiveListeners; got != 1 {
		t.Fatalf("ActiveListeners = %d, want 1", got)
	}

	unsubscribe()

	if got := r.Totals().ActiveListeners; got != 0 {
		t.Errorf("ActiveListeners after unsubscribe = %d, want 0", got)
	}
}

// The whole reason retired counters exist: closing a stream must not make a
// Prometheus counter go backwards. A falling counter is read as a process
// restart, and every rate() spanning that scrape is silently wrong.
func TestTotals_BytesSurviveTheStreamThatProducedThem(t *testing.T) {
	r := NewRegistry()
	h := r.Open(context.Background(), "s")

	if _, _, _, err := h.Subscribe(); err != nil {
		t.Fatalf("Subscribe() = %v", err)
	}
	if err := h.Publish([]byte("0123456789")); err != nil {
		t.Fatalf("Publish() = %v", err)
	}

	live := r.Totals()
	if live.BytesPublished != 10 {
		t.Fatalf("BytesPublished while live = %d, want 10", live.BytesPublished)
	}

	r.Close("s")

	after := r.Totals()
	if after.ActiveStreams != 0 {
		t.Errorf("ActiveStreams after close = %d, want 0", after.ActiveStreams)
	}
	if after.ActiveListeners != 0 {
		t.Errorf("ActiveListeners after close = %d, want 0", after.ActiveListeners)
	}
	if after.BytesPublished != 10 {
		t.Errorf("BytesPublished after close = %d, want 10 — the counter went backwards", after.BytesPublished)
	}
}

func TestTotals_BytesAccumulateAcrossStreams(t *testing.T) {
	r := NewRegistry()
	ctx := context.Background()

	for _, id := range []string{"a", "b", "c"} {
		h := r.Open(ctx, id)
		if _, _, _, err := h.Subscribe(); err != nil {
			t.Fatalf("Subscribe() = %v", err)
		}
		if err := h.Publish([]byte("1234")); err != nil {
			t.Fatalf("Publish() = %v", err)
		}
		r.Close(id)
	}

	if got := r.Totals().BytesPublished; got != 12 {
		t.Errorf("BytesPublished = %d, want 12", got)
	}
}

// Open on an already-live stream closes the previous hub. Its bytes must be
// folded in on the way out, exactly as an explicit Close would.
func TestTotals_ReopeningAStreamKeepsThePreviousBytes(t *testing.T) {
	r := NewRegistry()
	ctx := context.Background()

	first := r.Open(ctx, "s")
	if _, _, _, err := first.Subscribe(); err != nil {
		t.Fatalf("Subscribe() = %v", err)
	}
	if err := first.Publish([]byte("12345")); err != nil {
		t.Fatalf("Publish() = %v", err)
	}

	r.Open(ctx, "s") // broadcaster reconnects

	got := r.Totals()
	if got.ActiveStreams != 1 {
		t.Errorf("ActiveStreams = %d, want 1", got.ActiveStreams)
	}
	if got.BytesPublished != 5 {
		t.Errorf("BytesPublished = %d, want 5 — the replaced hub's bytes were lost", got.BytesPublished)
	}
}

func TestTotals_CloseAllRetainsCounters(t *testing.T) {
	r := NewRegistry()
	ctx := context.Background()

	for _, id := range []string{"a", "b"} {
		h := r.Open(ctx, id)
		if _, _, _, err := h.Subscribe(); err != nil {
			t.Fatalf("Subscribe() = %v", err)
		}
		if err := h.Publish([]byte("abc")); err != nil {
			t.Fatalf("Publish() = %v", err)
		}
	}

	r.CloseAll()

	got := r.Totals()
	if got.ActiveStreams != 0 {
		t.Errorf("ActiveStreams = %d, want 0", got.ActiveStreams)
	}
	if got.BytesPublished != 6 {
		t.Errorf("BytesPublished = %d, want 6", got.BytesPublished)
	}
}

// Publishing with nobody attached still moves bytes: the broadcaster is live
// and the hub is doing work, which is what the counter is meant to show.
func TestTotals_BytesCountedWithoutListeners(t *testing.T) {
	r := NewRegistry()
	h := r.Open(context.Background(), "s")

	if err := h.Publish([]byte("xyz")); err != nil {
		t.Fatalf("Publish() = %v", err)
	}

	got := r.Totals()
	if got.ActiveListeners != 0 {
		t.Errorf("ActiveListeners = %d, want 0", got.ActiveListeners)
	}
	if got.BytesPublished != 3 {
		t.Errorf("BytesPublished = %d, want 3", got.BytesPublished)
	}
}

// The sequential tests above all read Totals *after* Close has returned, so
// none of them observes the window this one targets: a scrape landing while a
// stream is being closed. Prometheus reads a counter that dips and recovers
// as a process restart, so one such sample poisons every rate() spanning it —
// and every panel of the streaming dashboard is built on rate().
func TestTotals_BytesNeverGoBackwardsWhileStreamsClose(t *testing.T) {
	const (
		streams   = 50
		perStream = 20
		chunk     = "0123456789"
	)

	r := NewRegistry()
	ctx := context.Background()

	stop := make(chan struct{})
	regression := make(chan int64, 1)

	// Stands in for Prometheus: reads the totals in a tight loop and reports
	// the first value it sees go backwards.
	go func() {
		defer close(regression)
		var prev int64
		for {
			select {
			case <-stop:
				return
			default:
			}
			b := r.Totals().BytesPublished
			if b < prev {
				select {
				case regression <- b:
				default:
				}
				return
			}
			prev = b
		}
	}()

	var published int64
	for i := 0; i < streams; i++ {
		id := string(rune('a' + i%26))
		h := r.Open(ctx, id)
		for j := 0; j < perStream; j++ {
			if err := h.Publish([]byte(chunk)); err != nil {
				t.Fatalf("Publish() = %v", err)
			}
			published += int64(len(chunk))
		}
		r.Close(id)
	}

	close(stop)
	if b, ok := <-regression; ok {
		t.Fatalf("a scrape read BytesPublished = %d after a higher value — the counter went backwards while a stream was closing", b)
	}

	if got := r.Totals().BytesPublished; got != published {
		t.Fatalf("final BytesPublished = %d, want %d", got, published)
	}
}

// A broadcast that starts and ends between two scrapes never shows up in the
// ActiveStreams gauge. Without this counter, "how many lives happened
// yesterday" is unanswerable — which is the question a product owner asks
// first.
func TestTotals_SessionsStartedSurviveTheStreamsThatEnded(t *testing.T) {
	r := NewRegistry()
	ctx := context.Background()

	for _, id := range []string{"a", "b", "c"} {
		r.Open(ctx, id)
		r.Close(id)
	}

	got := r.Totals()
	if got.ActiveStreams != 0 {
		t.Errorf("ActiveStreams = %d, want 0 — nothing is live any more", got.ActiveStreams)
	}
	if got.SessionsStarted != 3 {
		t.Errorf("SessionsStarted = %d, want 3", got.SessionsStarted)
	}
}

// A broadcaster reconnecting reopens the same stream id. That is a new
// session: the counter has to move, otherwise a flapping broadcaster looks
// like one uninterrupted live.
func TestTotals_ReopeningAStreamCountsANewSession(t *testing.T) {
	r := NewRegistry()
	ctx := context.Background()

	r.Open(ctx, "s")
	r.Open(ctx, "s")

	if got := r.Totals().SessionsStarted; got != 2 {
		t.Errorf("SessionsStarted = %d, want 2", got)
	}
	if got := r.Totals().ActiveStreams; got != 1 {
		t.Errorf("ActiveStreams = %d, want 1 — the reconnect replaced the hub", got)
	}
}
