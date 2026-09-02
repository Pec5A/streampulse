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
	}
	h := NewHub(ctx, streamID)
	r.hubs[streamID] = h
	return h
}

// Close ends a stream's live session and detaches its listeners.
func (r *Registry) Close(streamID string) {
	r.mu.Lock()
	h, ok := r.hubs[streamID]
	if ok {
		delete(r.hubs, streamID)
	}
	r.mu.Unlock()

	if ok {
		h.Close()
	}
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
	hubs := make([]*Hub, 0, len(r.hubs))
	for id, h := range r.hubs {
		hubs = append(hubs, h)
		delete(r.hubs, id)
	}
	r.mu.Unlock()

	for _, h := range hubs {
		h.Close()
	}
}
