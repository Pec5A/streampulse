package dto

import (
	"time"

	"github.com/streampulse/backend/internal/domain/entity"
)

type CreatePlaylistRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
}

// UpdatePlaylistRequest uses pointers so callers can PATCH a subset of fields
// (a nil field is left unchanged).
type UpdatePlaylistRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	IsPublic    *bool   `json:"is_public"`
}

type AddTrackRequest struct {
	Title           string `json:"title"`
	Artist          string `json:"artist"`
	DurationSeconds int    `json:"duration_seconds"`
	SourceURL       string `json:"source_url"`
}

type ReorderRequest struct {
	TrackIDs []string `json:"track_ids"`
}

type TrackResponse struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Artist          string `json:"artist"`
	DurationSeconds int    `json:"duration_seconds"`
	SourceURL       string `json:"source_url"`
	Position        int    `json:"position"`
}

type PlaylistResponse struct {
	ID          string `json:"id"`
	OwnerID     string `json:"owner_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type PlaylistWithTracksResponse struct {
	PlaylistResponse
	Tracks []TrackResponse `json:"tracks"`
}

func PlaylistFrom(p *entity.Playlist) PlaylistResponse {
	return PlaylistResponse{
		ID:          p.ID,
		OwnerID:     p.OwnerID,
		Name:        p.Name,
		Description: p.Description,
		IsPublic:    p.IsPublic,
		CreatedAt:   p.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   p.UpdatedAt.Format(time.RFC3339),
	}
}

func TrackFrom(t entity.Track) TrackResponse {
	return TrackResponse{
		ID:              t.ID,
		Title:           t.Title,
		Artist:          t.Artist,
		DurationSeconds: t.DurationSeconds,
		SourceURL:       t.SourceURL,
		Position:        t.Position,
	}
}

func TracksFrom(tracks []entity.Track) []TrackResponse {
	out := make([]TrackResponse, len(tracks))
	for i, t := range tracks {
		out[i] = TrackFrom(t)
	}
	return out
}

func PlaylistWithTracks(p *entity.Playlist, tracks []entity.Track) PlaylistWithTracksResponse {
	return PlaylistWithTracksResponse{
		PlaylistResponse: PlaylistFrom(p),
		Tracks:           TracksFrom(tracks),
	}
}
