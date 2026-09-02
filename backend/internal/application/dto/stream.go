package dto

import (
	"time"

	"github.com/streampulse/backend/internal/domain/entity"
)

type CreateStreamRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// StreamResponse is the wire shape of a stream. ListenerCount is not part of
// the entity: it is a live value read from the streaming registry at
// response time, which is why it is injected here rather than persisted.
type StreamResponse struct {
	ID                  string    `json:"id"`
	Title               string    `json:"title"`
	Description         string    `json:"description"`
	BroadcasterID       string    `json:"broadcaster_id"`
	BroadcasterUsername string    `json:"broadcaster_username,omitempty"`
	Status              string    `json:"status"`
	ListenerCount       int       `json:"listener_count"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// PublishSummary is returned to a broadcaster once its publish session ends.
// It is written after the fact on purpose — see StreamHandler.Publish.
type PublishSummary struct {
	StreamID       string `json:"stream_id"`
	BytesPublished int64  `json:"bytes_published"`
}

// StreamFrom projects an entity onto the wire shape. listenerCount comes from
// the caller because only the transport layer holds the registry.
func StreamFrom(s *entity.Stream, listenerCount int) StreamResponse {
	return StreamResponse{
		ID:                  s.ID,
		Title:               s.Title,
		Description:         s.Description,
		BroadcasterID:       s.BroadcasterID,
		BroadcasterUsername: s.BroadcasterUsername,
		Status:              string(s.Status),
		ListenerCount:       listenerCount,
		CreatedAt:           s.CreatedAt,
		UpdatedAt:           s.UpdatedAt,
	}
}
