package entity

import "time"

// Track is an audio file uploaded by a broadcaster.
//
// Note what is *not* here: no public URL. The reference implementation stored
// one, which bakes the host name into the database — every environment change
// (or a move behind a CDN) would then require rewriting rows. We store the
// opaque storage key and let the transport layer build the URL at read time.
type Track struct {
	ID string

	Title  string
	Artist string

	// StorageKey identifies the object in the storage port. It is generated
	// server-side and never derived from the uploaded filename.
	StorageKey string

	// Filename is the original client-supplied name, kept for display and
	// for the download filename only — never used to build a path.
	Filename    string
	ContentType string
	SizeBytes   int64

	UploaderID       string
	UploaderUsername string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// OwnedBy reports whether userID uploaded this track.
func (t *Track) OwnedBy(userID string) bool { return t.UploaderID == userID }
