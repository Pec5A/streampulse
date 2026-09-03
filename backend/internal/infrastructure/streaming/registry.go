package streaming

import (
	"context"
	"sync"
)

// Registry is the process-wide index of live hubs, keyed by stream id.
// Handlers never build a Hub themselves: they ask the registry, which is the
// single place where a stream's live lifetime begins and ends.
//
// Consequence worth stating explicitly (and covered in the ADR): the registry
// is per-process, so a multi-replica deployment would need listeners to reach
// the same replica as their broadcaster. That is a deliberate trade-off for
// this project's scale, not an oversight.
type Registry struct {
	mu   sync.RWMutex
	hubs map[string]*Hub

	// Final counters of hubs that have already closed.
	//
	// A hub's counters die with the hub, so a process-wide total computed only
	// from live hubs would *fall* every time a broadcast ended. Prometheus
	// reads a falling counter as a process restart and treats the drop as a
	// reset, which silently corrupts every rate() spanning that moment. Folding
	// each hub's final numbers in here on the way out keeps the totals
	// monotonic, which is the contract a counter has to honour.
	retiredBytes     int64
	retiredDropped   int64
	retiredEvictions int64

	// Every live session ever opened, including the ones already finished.
	// The gauge above answers "how many streams right now"; this answers
	// "how many broadcasts happened yesterday", which no gauge can — a
	// broadcast that started and ended between two scrapes leaves no trace
	// in a gauge at all.
	sessionsStarted int64
}

// Totals is a process-wide view of broadcasting activity: live gauges plus
// monotonic counters that survive the streams they came from.
type Totals struct {
	ActiveStreams   int
	ActiveListeners int
	SessionsStarted int64
	BytesPublished  int64
	ChunksDropped   int64
	Evictions       int64
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{hubs: make(map[string]*Hub)}
}

// Open starts a hub for streamID. If the stream was already live — a
// broadcaster reconnecting after a dropped connection, for instance — the
// previous hub is closed first so its listeners are released instead of
// being stranded on a hub nobody publishes to any more.
func (r *Registry) Open(ctx context.Context, streamID string) *Hub {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.hubs[streamID]; ok {
		existing.Close()
		// Already holding the write lock, so fold the counters in directly
		// rather than going through retire, which would deadlock.
		r.absorbLocked(existing)
	}
	h := NewHub(ctx, streamID)
	r.hubs[streamID] = h
	r.sessionsStarted++
	return h
}

// Close ends a stream's live session and detaches its listeners.
//
// Removing it from the map, closing it and absorbing its counters all happen
// under one write lock. Releasing the lock in between would open a window
// where the hub is no longer counted among the live ones and its bytes are
// not yet in the retired totals: a scrape landing there reads a counter that
// went backwards, which Prometheus takes for a process restart and which
// corrupts every rate() spanning that instant.
func (r *Registry) Close(streamID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	h, ok := r.hubs[streamID]
	if !ok {
		return
	}
	delete(r.hubs, streamID)
	h.Close()
	r.absorbLocked(h)
}

// absorbLocked adds a closed hub's counters to the retired totals. Caller
// holds the write lock.
//
// Only call it after h.Close(): a closed hub rejects further publishes and
// Close has drained the in-flight ones, so its counters are final — nothing
// lands after this read.
func (r *Registry) absorbLocked(h *Hub) {
	s := h.Stats()
	r.retiredBytes += s.BytesPublished
	r.retiredDropped += s.ChunksDropped
	r.retiredEvictions += s.Evictions
}

// Totals snapshots live gauges and monotonic counters in one pass.
//
// Listeners are counted from the live hubs only — a listener that has left is
// not active — while the byte, drop and eviction counters add the live hubs to
// everything already retired, so they only ever go up.
func (r *Registry) Totals() Totals {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t := Totals{
		ActiveStreams:   len(r.hubs),
		SessionsStarted: r.sessionsStarted,
		BytesPublished:  r.retiredBytes,
		ChunksDropped:   r.retiredDropped,
		Evictions:       r.retiredEvictions,
	}
	for _, h := range r.hubs {
		s := h.Stats()
		t.ActiveListeners += s.Listeners
		t.BytesPublished += s.BytesPublished
		t.ChunksDropped += s.ChunksDropped
		t.Evictions += s.Evictions
	}
	return t
}

// Get returns the live hub for streamID, or ErrStreamNotFound.
func (r *Registry) Get(streamID string) (*Hub, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	h, ok := r.hubs[streamID]
	if !ok {
		return nil, ErrStreamNotFound
	}
	return h, nil
}

// IsLive reports whether a stream currently has a hub.
func (r *Registry) IsLive(streamID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.hubs[streamID]
	return ok
}

// ListenerCount returns the live listener count for a stream, or 0 if the
// stream is not live. Handlers use it to decorate stream responses without
// having to handle a "not live" error on a read path.
func (r *Registry) ListenerCount(streamID string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	h, ok := r.hubs[streamID]
	if !ok {
		return 0
	}
	return h.ListenerCount()
}

// LiveIDs returns the ids of every currently live stream.
func (r *Registry) LiveIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.hubs))
	for id := range r.hubs {
		ids = append(ids, id)
	}
	return ids
}

// CloseAll shuts every hub down. Called on server shutdown so that in-flight
// listener goroutines terminate instead of being killed mid-write.
func (r *Registry) CloseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Same reason as Close: the lock is never released between removing a
	// hub from the map and absorbing its counters.
	for id, h := range r.hubs {
		delete(r.hubs, id)
		h.Close()
		r.absorbLocked(h)
	}
}
