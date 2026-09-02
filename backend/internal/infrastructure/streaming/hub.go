// Package streaming implements the in-memory pub/sub fan-out that carries
// audio chunks from one broadcaster to N simultaneous listeners.
//
// Design rationale: see docs/adr/0008-streaming-pubsub.md.
//
// Concurrency model
//
//   - The broadcaster's HTTP (or WebSocket) handler goroutine calls Publish.
//   - Each listener's handler goroutine owns one buffered channel and reads
//     from it; the hub never writes to the network itself.
//   - Back-pressure is per listener: a listener that cannot keep up loses
//     chunks, never the broadcaster and never its peers. Publish is
//     non-blocking by construction.
//   - Memory is bounded by design: listeners × ListenerBuffer × chunk size.
//     Nothing in the hub grows without a ceiling.
//   - Closing the hub cancels its context and closes every listener channel,
//     which unblocks and terminates every listener goroutine — no leaks.
//
// This package deliberately has no dependency on metrics or logging: it
// exposes counters through Stats() so that the transport layer (or a future
// Prometheus collector) decides what to record. That keeps the hub unit
// testable with nothing but the standard library.
package streaming

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

const (
	// ListenerBuffer is how many chunks a single listener may fall behind
	// before chunks start being dropped. At ~4 KB per chunk this caps a
	// listener's queue at ~1 MB, which absorbs normal network jitter without
	// letting a stalled client grow the heap indefinitely.
	ListenerBuffer = 256

	// MaxConsecutiveDrops is how many chunks in a row a listener may miss
	// before the hub evicts it. A single dropped chunk is transient jitter
	// and recovers on its own; a listener that misses this many in a row is
	// gone (dead TCP connection, suspended app) and is only costing memory.
	MaxConsecutiveDrops = 64
)

var (
	// ErrStreamNotFound is returned when no hub exists for a stream id.
	ErrStreamNotFound = errors.New("stream not found")
	// ErrStreamClosed is returned when publishing to or subscribing to a
	// hub that has already been closed.
	ErrStreamClosed = errors.New("stream closed")
)

// Eviction reasons reported through Stats and to unsubscribe callers.
const (
	reasonClientClose  = "client_close"
	reasonSlowConsumer = "slow_consumer"
)

// subscriber is one listener's mailbox.
type subscriber struct {
	ch chan []byte
	// drops counts *consecutive* failed deliveries. It is reset on every
	// successful send, so it measures "is this listener stuck right now",
	// not "has it ever hiccuped".
	drops atomic.Int32
	// evicting guarantees a single eviction goroutine per subscriber even
	// if several publishes observe the threshold at once.
	evicting atomic.Bool
}

// Stats is a point-in-time snapshot of a hub's activity, used by handlers
// and by tests to assert on fan-out behaviour.
type Stats struct {
	Listeners      int
	BytesPublished int64
	ChunksDropped  int64
	Evictions      int64
}

// Hub fans one publisher out to many subscribers. It is safe for concurrent
// use by any number of goroutines.
type Hub struct {
	streamID string
	bufSize  int
	maxDrops int32

	mu     sync.RWMutex
	subs   map[string]*subscriber
	closed bool

	listeners      atomic.Int64
	bytesPublished atomic.Int64
	chunksDropped  atomic.Int64
	evictions      atomic.Int64

	ctx    context.Context
	cancel context.CancelFunc
}

// NewHub creates a hub for streamID. Cancelling parent closes the hub and
// every listener attached to it.
func NewHub(parent context.Context, streamID string) *Hub {
	ctx, cancel := context.WithCancel(parent)
	return &Hub{
		streamID: streamID,
		bufSize:  ListenerBuffer,
		maxDrops: MaxConsecutiveDrops,
		subs:     make(map[string]*subscriber),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// StreamID returns the stream this hub serves.
func (h *Hub) StreamID() string { return h.streamID }

// Subscribe attaches a listener. The returned channel delivers audio chunks
// and is closed when the listener is evicted, unsubscribes, or the hub
// closes — so a `for chunk := range ch` loop always terminates.
//
// The caller must invoke the returned unsubscribe function (typically via
// defer) to release the listener's buffer.
func (h *Hub) Subscribe() (id string, chunks <-chan []byte, unsubscribe func(), err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return "", nil, nil, ErrStreamClosed
	}

	id = uuid.NewString()
	sub := &subscriber{ch: make(chan []byte, h.bufSize)}
	h.subs[id] = sub
	h.listeners.Add(1)

	return id, sub.ch, func() { h.remove(id, reasonClientClose) }, nil
}

// Publish fans a chunk out to every current subscriber and never blocks.
//
// The chunk is copied once: the broadcaster's read buffer is reused between
// reads, so handing the same backing array to listeners would let a slow
// listener observe a chunk that has since been overwritten. One copy per
// publish (not per listener) is the cheapest way to make the payload
// immutable for everyone.
func (h *Hub) Publish(chunk []byte) error {
	h.mu.RLock()

	if h.closed {
		h.mu.RUnlock()
		return ErrStreamClosed
	}

	buf := make([]byte, len(chunk))
	copy(buf, chunk)

	for id, sub := range h.subs {
		select {
		case sub.ch <- buf:
			sub.drops.Store(0)
		default:
			h.chunksDropped.Add(1)
			if sub.drops.Add(1) >= h.maxDrops && sub.evicting.CompareAndSwap(false, true) {
				// remove takes the write lock, so it has to run outside this
				// critical section — hence the goroutine.
				go h.remove(id, reasonSlowConsumer)
			}
		}
	}

	h.mu.RUnlock()

	h.bytesPublished.Add(int64(len(chunk)))
	return nil
}

// Close terminates the hub: every listener channel is closed and the hub's
// context is cancelled. Safe to call more than once.
//
// Close takes the write lock, so it cannot run while a Publish is mid-fan-out
// — which is what guarantees no goroutine ever sends on a closed channel.
func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	for id, sub := range h.subs {
		close(sub.ch)
		delete(h.subs, id)
	}
	h.listeners.Store(0)
	h.mu.Unlock()

	h.cancel()
}

// ListenerCount returns how many listeners are currently attached.
func (h *Hub) ListenerCount() int { return int(h.listeners.Load()) }

// Stats snapshots the hub's counters.
func (h *Hub) Stats() Stats {
	return Stats{
		Listeners:      h.ListenerCount(),
		BytesPublished: h.bytesPublished.Load(),
		ChunksDropped:  h.chunksDropped.Load(),
		Evictions:      h.evictions.Load(),
	}
}

// Done is closed when the hub is closed or its parent context is cancelled.
// Listener handlers select on it to stop as soon as the broadcaster leaves.
func (h *Hub) Done() <-chan struct{} { return h.ctx.Done() }

// remove detaches one subscriber. It is idempotent: unsubscribing twice, or
// unsubscribing a listener the hub already evicted, is a no-op.
func (h *Hub) remove(id, reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	sub, ok := h.subs[id]
	if !ok {
		return
	}
	close(sub.ch)
	delete(h.subs, id)
	h.listeners.Add(-1)
	if reason == reasonSlowConsumer {
		h.evictions.Add(1)
	}
}
