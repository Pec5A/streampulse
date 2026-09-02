package streaming

import (
	"context"
	"runtime"
	"sync"
	"testing"
	"time"
)

// recvWithin reads one chunk or fails the test after d.
func recvWithin(t *testing.T, ch <-chan []byte, d time.Duration) []byte {
	t.Helper()
	select {
	case chunk, ok := <-ch:
		if !ok {
			t.Fatal("channel closed while a chunk was expected")
		}
		return chunk
	case <-time.After(d):
		t.Fatal("timed out waiting for a chunk")
		return nil
	}
}

func TestHub_FanOutToManySimultaneousListeners(t *testing.T) {
	// The headline requirement of K1: one broadcaster, N listeners, every
	// listener receives every chunk.
	const (
		listeners = 100
		chunks    = 50
	)

	hub := NewHub(context.Background(), "stream-1")
	defer hub.Close()

	var wg sync.WaitGroup
	received := make([]int, listeners)

	for i := range listeners {
		_, ch, unsub, err := hub.Subscribe()
		if err != nil {
			t.Fatalf("subscribe %d: %v", i, err)
		}
		wg.Add(1)
		go func(idx int, ch <-chan []byte, unsub func()) {
			defer wg.Done()
			defer unsub()
			for range chunks {
				select {
				case _, ok := <-ch:
					if !ok {
						return
					}
					received[idx]++
				case <-time.After(5 * time.Second):
					return
				}
			}
		}(i, ch, unsub)
	}

	if got := hub.ListenerCount(); got != listeners {
		t.Fatalf("ListenerCount = %d, want %d", got, listeners)
	}

	for i := range chunks {
		if err := hub.Publish([]byte{byte(i)}); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}

	wg.Wait()

	for i, n := range received {
		if n != chunks {
			t.Errorf("listener %d received %d chunks, want %d", i, n, chunks)
		}
	}

	if stats := hub.Stats(); stats.ChunksDropped != 0 {
		t.Errorf("ChunksDropped = %d, want 0 (buffers are large enough here)", stats.ChunksDropped)
	}
}

func TestHub_PublishNeverBlocksOnASlowListener(t *testing.T) {
	// A listener that never reads must not be able to stall the broadcaster.
	hub := NewHub(context.Background(), "stream-slow")
	defer hub.Close()

	_, _, _, err := hub.Subscribe() // deliberately never drained
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Far more chunks than the listener's buffer can hold.
		for i := range ListenerBuffer * 4 {
			_ = hub.Publish([]byte{byte(i)})
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Publish blocked on a listener that stopped reading")
	}

	if stats := hub.Stats(); stats.ChunksDropped == 0 {
		t.Error("expected dropped chunks for a listener that never reads")
	}
}

func TestHub_EvictsListenerAfterConsecutiveDrops(t *testing.T) {
	hub := NewHub(context.Background(), "stream-evict")
	defer hub.Close()

	_, ch, _, err := hub.Subscribe()
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	// Fill the buffer, then keep publishing so drops accumulate past the
	// eviction threshold.
	for range ListenerBuffer + MaxConsecutiveDrops + 1 {
		_ = hub.Publish([]byte("x"))
	}

	// Eviction happens in its own goroutine; wait for the channel to close.
	deadline := time.After(2 * time.Second)
	for {
		if hub.ListenerCount() == 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("slow listener was never evicted")
		case <-time.After(5 * time.Millisecond):
		}
	}

	// Draining the evicted listener must terminate: its channel is closed.
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for range ch { //nolint:revive // draining until close is the point
		}
	}()
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("evicted listener's channel was not closed")
	}

	if stats := hub.Stats(); stats.Evictions != 1 {
		t.Errorf("Evictions = %d, want 1", stats.Evictions)
	}
}

func TestHub_ASlowListenerDoesNotPenaliseItsPeers(t *testing.T) {
	hub := NewHub(context.Background(), "stream-mixed")
	defer hub.Close()

	if _, _, _, err := hub.Subscribe(); err != nil { // stalled listener
		t.Fatalf("subscribe stalled: %v", err)
	}
	_, fastCh, unsub, err := hub.Subscribe()
	if err != nil {
		t.Fatalf("subscribe fast: %v", err)
	}
	defer unsub()

	const chunks = 20
	go func() {
		for i := range chunks {
			_ = hub.Publish([]byte{byte(i)})
		}
	}()

	for i := range chunks {
		got := recvWithin(t, fastCh, 2*time.Second)
		if len(got) != 1 || got[0] != byte(i) {
			t.Fatalf("chunk %d = %v, want [%d]", i, got, i)
		}
	}
}

func TestHub_PublishCopiesTheCallersBuffer(t *testing.T) {
	// The broadcaster handler reuses one read buffer across reads. If the hub
	// handed that array straight to listeners, a listener reading later would
	// see whatever the next read overwrote it with.
	hub := NewHub(context.Background(), "stream-copy")
	defer hub.Close()

	_, ch, unsub, err := hub.Subscribe()
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsub()

	shared := []byte("first")
	if err := hub.Publish(shared); err != nil {
		t.Fatalf("publish: %v", err)
	}
	copy(shared, "sssss") // simulate the next read overwriting the buffer

	got := recvWithin(t, ch, time.Second)
	if string(got) != "first" {
		t.Errorf("listener saw %q, want %q — the hub aliased the caller's buffer", got, "first")
	}
}

func TestHub_UnsubscribeClosesChannelAndIsIdempotent(t *testing.T) {
	hub := NewHub(context.Background(), "stream-unsub")
	defer hub.Close()

	_, ch, unsub, err := hub.Subscribe()
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	unsub()
	unsub() // must not panic on a double close

	if _, ok := <-ch; ok {
		t.Error("channel should be closed after unsubscribe")
	}
	if got := hub.ListenerCount(); got != 0 {
		t.Errorf("ListenerCount = %d, want 0", got)
	}
}

func TestHub_CloseReleasesEveryListener(t *testing.T) {
	hub := NewHub(context.Background(), "stream-close")

	const listeners = 10
	channels := make([]<-chan []byte, listeners)
	for i := range listeners {
		_, ch, _, err := hub.Subscribe()
		if err != nil {
			t.Fatalf("subscribe %d: %v", i, err)
		}
		channels[i] = ch
	}

	hub.Close()
	hub.Close() // idempotent

	for i, ch := range channels {
		select {
		case _, ok := <-ch:
			if ok {
				t.Errorf("listener %d channel still open after Close", i)
			}
		case <-time.After(time.Second):
			t.Errorf("listener %d channel was not closed by Close", i)
		}
	}

	select {
	case <-hub.Done():
	case <-time.After(time.Second):
		t.Error("Done() was not closed by Close")
	}
}

func TestHub_RejectsUseAfterClose(t *testing.T) {
	hub := NewHub(context.Background(), "stream-after-close")
	hub.Close()

	if err := hub.Publish([]byte("x")); err != ErrStreamClosed {
		t.Errorf("Publish after close = %v, want ErrStreamClosed", err)
	}
	if _, _, _, err := hub.Subscribe(); err != ErrStreamClosed {
		t.Errorf("Subscribe after close = %v, want ErrStreamClosed", err)
	}
}

func TestHub_ParentContextCancellationClosesTheHub(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	hub := NewHub(ctx, "stream-ctx")
	defer hub.Close()

	cancel()

	select {
	case <-hub.Done():
	case <-time.After(time.Second):
		t.Fatal("cancelling the parent context did not close Done()")
	}
}

func TestHub_DoesNotLeakGoroutines(t *testing.T) {
	// Memory/goroutine hygiene is an explicit K1 acceptance criterion:
	// churning listeners must not accumulate goroutines.
	before := runtime.NumGoroutine()

	for range 50 {
		hub := NewHub(context.Background(), "stream-leak")
		for range 20 {
			_, _, unsub, err := hub.Subscribe()
			if err != nil {
				t.Fatalf("subscribe: %v", err)
			}
			_ = hub.Publish([]byte("chunk"))
			unsub()
		}
		hub.Close()
	}

	// Give any eviction goroutines a chance to finish.
	deadline := time.Now().Add(2 * time.Second)
	var after int
	for time.Now().Before(deadline) {
		runtime.GC()
		after = runtime.NumGoroutine()
		if after <= before+5 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("goroutines grew from %d to %d — listeners are leaking", before, after)
}

func TestHub_ConcurrentSubscribePublishUnsubscribe(t *testing.T) {
	// Run under -race: this is the test that proves the locking is sound.
	hub := NewHub(context.Background(), "stream-race")
	defer hub.Close()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = hub.Publish([]byte("chunk"))
			}
		}
	}()

	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				id, ch, unsub, err := hub.Subscribe()
				if err != nil {
					return
				}
				_ = id
				select {
				case <-ch:
				default:
				}
				unsub()
			}
		}()
	}

	// Let the publisher run alongside the churn, then stop it.
	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()

	if got := hub.ListenerCount(); got != 0 {
		t.Errorf("ListenerCount = %d after all listeners unsubscribed, want 0", got)
	}
}

func TestHub_StatsCountPublishedBytes(t *testing.T) {
	hub := NewHub(context.Background(), "stream-stats")
	defer hub.Close()

	_, ch, unsub, err := hub.Subscribe()
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer unsub()

	for range 3 {
		if err := hub.Publish([]byte("1234")); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}
	for range 3 {
		recvWithin(t, ch, time.Second)
	}

	if got := hub.StreamID(); got != "stream-stats" {
		t.Errorf("StreamID() = %q, want stream-stats", got)
	}

	stats := hub.Stats()
	if stats.BytesPublished != 12 {
		t.Errorf("BytesPublished = %d, want 12", stats.BytesPublished)
	}
	if stats.Listeners != 1 {
		t.Errorf("Listeners = %d, want 1", stats.Listeners)
	}
}
