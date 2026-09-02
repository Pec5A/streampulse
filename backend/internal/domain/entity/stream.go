package entity

import "time"

// StreamStatus reflects whether a broadcaster is currently pushing audio.
// It is persisted so that a listener can discover live streams without the
// API having to expose its in-memory streaming registry.
type StreamStatus string

const (
	StreamStatusLive    StreamStatus = "live"
	StreamStatusOffline StreamStatus = "offline"
)

// Stream is a broadcast channel owned by one user (its broadcaster).
// It carries only persisted state: the live listener count is a runtime
// value held by the streaming hub, so it belongs to the DTO layer, not here.
type Stream struct {
	ID            string
	Title         string
	Description   string
	BroadcasterID string
	// BroadcasterUsername is filled by repository joins for display; it is
	// not a column of the streams table.
	BroadcasterUsername string
	Status              StreamStatus
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// IsLive reports whether the stream is flagged as broadcasting.
func (s *Stream) IsLive() bool { return s.Status == StreamStatusLive }

// OwnedBy reports whether userID is the stream's broadcaster.
func (s *Stream) OwnedBy(userID string) bool { return s.BroadcasterID == userID }
