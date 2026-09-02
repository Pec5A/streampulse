package repository

import (
	"context"

	"github.com/streampulse/backend/internal/domain/entity"
)

// PlaylistRepository persists playlists and their ordered tracks. Track
// positions are expected to stay contiguous (0..n-1) after every mutation,
// and ReorderTracks must apply atomically.
//
// Implementations return ErrNotFound / ErrConflict (see errors.go) rather
// than storage-specific errors, so the use case stays persistence-agnostic.
type PlaylistRepository interface {
	Create(ctx context.Context, p *entity.Playlist) error
	FindByID(ctx context.Context, id string) (*entity.Playlist, error)
	ListByOwner(ctx context.Context, ownerID string) ([]entity.Playlist, error)
	Update(ctx context.Context, p *entity.Playlist) error
	Delete(ctx context.Context, id string) error

	// ListTracks returns the playlist's tracks ordered by position.
	ListTracks(ctx context.Context, playlistID string) ([]entity.Track, error)
	// AddTrack appends t to the end of the playlist's queue, assigning its
	// Position and ID.
	AddTrack(ctx context.Context, t *entity.Track) error
	// RemoveTrack deletes the track and compacts the remaining positions.
	RemoveTrack(ctx context.Context, playlistID, trackID string) error
	// ReorderTracks atomically sets each listed track's position to its index
	// in orderedTrackIDs. The slice is expected to be exactly the playlist's
	// current track set (the use case validates this beforehand).
	ReorderTracks(ctx context.Context, playlistID string, orderedTrackIDs []string) error
}
