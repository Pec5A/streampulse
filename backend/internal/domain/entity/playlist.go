package entity

import "time"

// Playlist is a user-owned, ordered collection of tracks (a play queue).
type Playlist struct {
	ID          string
	OwnerID     string
	Name        string
	Description string
	IsPublic    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Track is a single entry in a playlist's queue. Tracks are owned by their
// playlist (not a shared catalog), which keeps this slice independent of the
// upload/streaming features built by other tickets. SourceURL is an optional,
// forward-compatible pointer to the media those features will later provide.
type Track struct {
	ID              string
	PlaylistID      string
	Title           string
	Artist          string
	DurationSeconds int
	SourceURL       string
	Position        int
	CreatedAt       time.Time
}
