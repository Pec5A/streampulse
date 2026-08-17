// Package storage implements the domain storage port on the local
// filesystem. It is the development and single-node default; swapping in S3
// means writing one more type that satisfies domainstorage.Storage.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	domainstorage "github.com/streampulse/backend/internal/domain/storage"
)

// dirPerm/filePerm keep uploads readable by the service account only.
const (
	dirPerm  os.FileMode = 0o750
	filePerm os.FileMode = 0o640
)

// ErrInvalidKey is returned for a key that would escape the base directory.
var ErrInvalidKey = errors.New("invalid storage key")

// Local stores objects as files under BasePath.
type Local struct {
	basePath string
}

// NewLocal creates the base directory if needed.
func NewLocal(basePath string) (*Local, error) {
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("resolve storage path: %w", err)
	}
	if err := os.MkdirAll(abs, dirPerm); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	return &Local{basePath: abs}, nil
}

var _ domainstorage.Storage = (*Local)(nil)

// resolve turns a key into an absolute path confined to basePath.
//
// This is the security boundary of the whole package. Even though keys are
// generated server-side today, a key that reaches here from user input must
// never be able to write outside the upload directory — so traversal is
// rejected structurally rather than by trusting the caller.
func (l *Local) resolve(key string) (string, error) {
	if key == "" || strings.ContainsRune(key, 0) {
		return "", ErrInvalidKey
	}
	// Reject anything that is not a plain, single-segment name.
	if filepath.IsAbs(key) || strings.ContainsAny(key, `/\`) {
		return "", ErrInvalidKey
	}
	if key == "." || key == ".." {
		return "", ErrInvalidKey
	}

	full := filepath.Join(l.basePath, key)

	// Belt and braces: even after the checks above, verify the result really
	// is inside basePath.
	rel, err := filepath.Rel(l.basePath, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrInvalidKey
	}
	return full, nil
}

func (l *Local) Save(_ context.Context, key string, r io.Reader) (int64, error) {
	path, err := l.resolve(key)
	if err != nil {
		return 0, err
	}

	// Write to a temporary file first, then rename: a crash mid-upload
	// leaves no half-written object that a listener could later stream as
	// if it were complete. Rename is atomic within a filesystem.
	tmp, err := os.CreateTemp(l.basePath, ".upload-*")
	if err != nil {
		return 0, fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName) // no-op once the rename succeeded
	}()

	written, err := io.Copy(tmp, r)
	if err != nil {
		return 0, fmt.Errorf("write object: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return 0, fmt.Errorf("sync object: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("close object: %w", err)
	}
	if err := os.Chmod(tmpName, filePerm); err != nil {
		return 0, fmt.Errorf("chmod object: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return 0, fmt.Errorf("commit object: %w", err)
	}
	return written, nil
}

func (l *Local) Open(_ context.Context, key string) (io.ReadSeekCloser, error) {
	path, err := l.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path) //nolint:gosec // path is confined by resolve
	if errors.Is(err, os.ErrNotExist) {
		return nil, domainstorage.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open object: %w", err)
	}
	return f, nil
}

func (l *Local) Delete(_ context.Context, key string) error {
	path, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domainstorage.ErrNotFound
		}
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

func (l *Local) Exists(_ context.Context, key string) (bool, error) {
	path, err := l.resolve(key)
	if err != nil {
		return false, err
	}
	switch _, err := os.Stat(path); {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("stat object: %w", err)
	}
}
