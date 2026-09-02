package dto

import (
	"time"

	"github.com/streampulse/backend/internal/domain/entity"
)

// AudioTrackResponse is the wire shape of an uploaded track.
//
// `storage_key` is deliberately absent: it is an internal handle, and leaking
// it would invite clients to build their own storage URLs instead of going
// through the API. `audio_url` is built at read time from the track id, so
// nothing about the host is ever persisted.
type AudioTrackResponse struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Artist           string    `json:"artist"`
	Filename         string    `json:"filename,omitempty"`
	ContentType      string    `json:"content_type"`
	SizeBytes        int64     `json:"size_bytes"`
	UploaderID       string    `json:"uploader_id"`
	UploaderUsername string    `json:"uploader_username,omitempty"`
	AudioURL         string    `json:"audio_url"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// AudioTrackFrom projects an entity onto the wire shape.
func AudioTrackFrom(t *entity.AudioTrack) AudioTrackResponse {
	return AudioTrackResponse{
		ID:               t.ID,
		Title:            t.Title,
		Artist:           t.Artist,
		Filename:         t.Filename,
		ContentType:      t.ContentType,
		SizeBytes:        t.SizeBytes,
		UploaderID:       t.UploaderID,
		UploaderUsername: t.UploaderUsername,
		AudioURL:         "/api/v1/tracks/" + t.ID + "/audio",
		CreatedAt:        t.CreatedAt,
		UpdatedAt:        t.UpdatedAt,
	}
}

// AudioTracksFrom projects a slice, always yielding [] rather than null so the
// Flutter decoder never has to special-case a missing list.
func AudioTracksFrom(tracks []entity.AudioTrack) []AudioTrackResponse {
	out := make([]AudioTrackResponse, 0, len(tracks))
	for i := range tracks {
		out = append(out, AudioTrackFrom(&tracks[i]))
	}
	return out
}
