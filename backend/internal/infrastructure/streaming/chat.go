// Chat is the in-memory pub/sub hub that fans text chat messages out to
// every participant of one stream's chat room.
//
// It deliberately mirrors Hub's design (see hub.go): same non-blocking
// publish, same bounded-channel backpressure policy, same per-stream
// registry with an Open/Close/Get lifecycle. The one structural difference
// is fan-out shape — Hub is 1 broadcaster to N listeners, ChatHub is N to N:
// any participant may publish, and every participant (including the sender)
// receives every message, so all clients render the same transcript.
//
// Chat has no eviction policy, unlike Hub. A stalled audio listener wastes
// memory indefinitely if never evicted, which is why Hub tracks consecutive
// drops. A stalled chat participant's dropped messages cost at most
// ChatBuffer small JSON payloads — bounded, not worth the extra mechanism.
package streaming

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// ChatBuffer is how many messages a single participant's channel holds
// before new messages are dropped for them specifically. Chat messages are
// small and infrequent compared to audio chunks, so a short buffer already
// absorbs normal bursts.
const ChatBuffer = 32

// ErrChatClosed is returned when publishing to, or subscribing on, a chat
// room that has already been closed.
var ErrChatClosed = errors.New("chat closed")

// ChatMessage is broadcast to every participant of a stream's chat room.
type ChatMessage struct {
	ID       string    `json:"id"`
	StreamID string    `json:"stream_id"`
	UserID   string    `json:"user_id"`
	Username string    `json:"username"`
	Text     string    `json:"text"`
	SentAt   time.Time `json:"sent_at"`
}

// ChatStats is a point-in-time snapshot of a chat room's activity.
type ChatStats struct {
	Participants int
	Messages     int64
	Dropped      int64
}

// ChatHub fans text messages out N-to-N between every participant of one
// stream's chat room. Safe for concurrent use by any number of goroutines.
type ChatHub struct {
	streamID string

	mu     sync.RWMutex
	subs   map[string]chan ChatMessage
	closed bool

	participants atomic.Int64
	messages     atomic.Int64
	dropped      atomic.Int64

	ctx    context.Context
	cancel context.CancelFunc
}

// NewChatHub creates a chat room for streamID. Cancelling parent closes the
// room and every participant attached to it.
func NewChatHub(parent context.Context, streamID string) *ChatHub {
	ctx, cancel := context.WithCancel(parent)
	return &ChatHub{
		streamID: streamID,
		subs:     make(map[string]chan ChatMessage),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// StreamID returns the stream this chat room belongs to.
func (h *ChatHub) StreamID() string { return h.streamID }

// Join attaches a participant. The returned channel delivers every message
// published to the room (including the participant's own) and is closed
// when the participant leaves or the room closes — so a `for msg := range
// ch` loop always terminates.
//
// The caller must invoke the returned leave function (typically via defer)
// to release the participant's buffer.
func (h *ChatHub) Join() (id string, messages <-chan ChatMessage, leave func(), err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return "", nil, nil, ErrChatClosed
	}

	id = uuid.NewString()
	ch := make(chan ChatMessage, ChatBuffer)
	h.subs[id] = ch
	h.participants.Add(1)

	return id, ch, func() { h.remove(id) }, nil
}

// Publish broadcasts a message to every current participant, including the
// sender. A slow participant has the message dropped for them rather than
// blocking the sender or their peers — the same backpressure policy as
// Hub.Publish.
func (h *ChatHub) Publish(msg ChatMessage) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if h.closed {
		return ErrChatClosed
	}

	for _, ch := range h.subs {
		select {
		case ch <- msg:
		default:
			h.dropped.Add(1)
		}
	}
	h.messages.Add(1)
	return nil
}

// Close terminates the room: every participant channel is closed and the
// room's context is cancelled. Safe to call more than once.
func (h *ChatHub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	for id, ch := range h.subs {
		close(ch)
		delete(h.subs, id)
	}
	h.participants.Store(0)
	h.mu.Unlock()

	h.cancel()
}

// Done is closed when the room is closed or its parent context is
// cancelled. Handlers select on it to stop as soon as the stream ends.
func (h *ChatHub) Done() <-chan struct{} { return h.ctx.Done() }

// Stats snapshots the room's counters.
func (h *ChatHub) Stats() ChatStats {
	return ChatStats{
		Participants: int(h.participants.Load()),
		Messages:     h.messages.Load(),
		Dropped:      h.dropped.Load(),
	}
}

func (h *ChatHub) remove(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch, ok := h.subs[id]
	if !ok {
		return
	}
	close(ch)
	delete(h.subs, id)
	h.participants.Add(-1)
}

// ChatRegistry is the process-wide index of live chat rooms, keyed by
// stream id. Mirrors Registry's lifecycle exactly (see registry.go): a room
// only exists while its stream is live, and StreamUseCase is the only
// caller that opens or closes one, in lockstep with the audio hub.
type ChatRegistry struct {
	mu   sync.RWMutex
	hubs map[string]*ChatHub
}

// NewChatRegistry returns an empty registry.
func NewChatRegistry() *ChatRegistry {
	return &ChatRegistry{hubs: make(map[string]*ChatHub)}
}

// Open starts a chat room for streamID. If a room already existed — a
// broadcaster reconnecting after a dropped connection, mirroring
// Registry.Open — the previous one is closed first so its participants are
// released instead of being stranded on a room nobody will ever publish a
// stream-end event to.
func (r *ChatRegistry) Open(ctx context.Context, streamID string) *ChatHub {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.hubs[streamID]; ok {
		existing.Close()
	}
	h := NewChatHub(ctx, streamID)
	r.hubs[streamID] = h
	return h
}

// Close ends a stream's chat room and detaches its participants.
func (r *ChatRegistry) Close(streamID string) {
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

// Get returns the chat room for streamID, or ErrStreamNotFound.
func (r *ChatRegistry) Get(streamID string) (*ChatHub, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	h, ok := r.hubs[streamID]
	if !ok {
		return nil, ErrStreamNotFound
	}
	return h, nil
}

// CloseAll shuts every chat room down. Called on server shutdown so that
// in-flight participant goroutines terminate instead of being killed
// mid-write.
func (r *ChatRegistry) CloseAll() {
	r.mu.Lock()
	hubs := make([]*ChatHub, 0, len(r.hubs))
	for id, h := range r.hubs {
		hubs = append(hubs, h)
		delete(r.hubs, id)
	}
	r.mu.Unlock()

	for _, h := range hubs {
		h.Close()
	}
}
