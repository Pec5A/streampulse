// Package storage defines where uploaded audio files live, as a port the
// application depends on rather than a concrete filesystem.
//
// The local implementation (internal/infrastructure/storage) is the default;
// an S3-backed one can replace it without touching a single use case. That is
// the whole point of keeping this interface narrow: four methods, no leaking
// of file paths, no os.File in the signatures.
package storage

import (
	"context"
	"errors"
	"io"
)

// ErrNotFound is returned when a key has no stored object.
var ErrNotFound = errors.New("object not found")

// Storage persists opaque byte streams under caller-chosen keys.
type Storage interface {
	// Save streams r into key and reports how many bytes were written.
	// Implementations must not buffer the whole payload in memory.
	Save(ctx context.Context, key string, r io.Reader) (int64, error)

	// Open returns a readable, seekable handle on the stored object.
	//
	// Seekable, not just readable, on purpose: it lets the HTTP layer answer
	// Range requests with http.ServeContent, which is what allows the mobile
	// player to seek inside an uploaded track instead of re-downloading it
	// from the start. An S3 implementation satisfies this with ranged GETs.
	Open(ctx context.Context, key string) (io.ReadSeekCloser, error)

	// Delete removes an object. Deleting a missing key returns ErrNotFound.
	Delete(ctx context.Context, key string) error

	// Exists reports whether a key is present, without opening it.
	Exists(ctx context.Context, key string) (bool, error)
}
