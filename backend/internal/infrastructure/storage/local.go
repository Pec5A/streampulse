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

	"github.com/google/uuid"

	domainstorage "github.com/streampulse/backend/internal/domain/storage"
)

// dirPerm/filePerm keep uploads readable by the service account only.
const (
	dirPerm  os.FileMode = 0o750
	filePerm os.FileMode = 0o640
)

// ErrInvalidKey is returned for a key that would escape the base directory.
var ErrInvalidKey = errors.New("invalid storage key")

// Local stores objects as files under a directory, accessed through an
// *os.Root so the confinement is enforced by the operating system rather than
// by string checks of our own.
type Local struct {
	basePath string
	root     *os.Root
}

// NewLocal creates the base directory if needed and opens it as a root.
func NewLocal(basePath string) (*Local, error) {
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("resolve storage path: %w", err)
	}
	if err := os.MkdirAll(abs, dirPerm); err != nil {
		return nil, fmt.Errorf("create storage dir: %w", err)
	}
	// os.Root holds a descriptor on the directory: every subsequent Open,
	// Create, Rename and Remove is resolved relative to it and refuses to
	// escape, symlinks included. That is a kernel-enforced boundary, not a
	// prefix comparison we could get subtly wrong.
	root, err := os.OpenRoot(abs)
	if err != nil {
		return nil, fmt.Errorf("open storage root: %w", err)
	}
	return &Local{basePath: abs, root: root}, nil
}

// Close releases the descriptor held on the base directory.
func (l *Local) Close() error { return l.root.Close() }

var _ domainstorage.Storage = (*Local)(nil)

// validateKey rejects anything that is not a plain, single-segment name.
//
// os.Root already makes escaping the base directory impossible, so this is no
// longer the security boundary — it is the *contract*: a storage key names one
// object, not a path. Keeping it means a malformed key fails with a clear
// ErrInvalidKey instead of a filesystem error, and it keeps the door shut on
// platforms where os.Root is documented as weaker (js, plan9).
func validateKey(key string) error {
	if key == "" || strings.ContainsRune(key, 0) {
		return ErrInvalidKey
	}
	if filepath.IsAbs(key) || strings.ContainsAny(key, `/\`) {
		return ErrInvalidKey
	}
	if key == "." || key == ".." {
		return ErrInvalidKey
	}
	return nil
}

func (l *Local) Save(_ context.Context, key string, r io.Reader) (int64, error) {
	if err := validateKey(key); err != nil {
		return 0, err
	}

	// Write to a temporary name first, then rename: a crash mid-upload leaves
	// no half-written object that a listener could later stream as if it were
	// complete. Rename is atomic within a filesystem, and os.Root.Rename keeps
	// both names confined to the base directory.
	tmp := ".upload-" + uuid.NewString()
	f, err := l.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
	if err != nil {
		return 0, fmt.Errorf("create temp object: %w", err)
	}
	committed := false
	defer func() {
		_ = f.Close()
		if !committed {
			_ = l.root.Remove(tmp)
		}
	}()

	written, err := io.Copy(f, r)
	if err != nil {
		return 0, fmt.Errorf("write object: %w", err)
	}
	if err := f.Sync(); err != nil {
		return 0, fmt.Errorf("sync object: %w", err)
	}
	if err := f.Close(); err != nil {
		return 0, fmt.Errorf("close object: %w", err)
	}
	if err := l.root.Rename(tmp, key); err != nil {
		return 0, fmt.Errorf("commit object: %w", err)
	}
	committed = true
	return written, nil
}

func (l *Local) Open(_ context.Context, key string) (io.ReadSeekCloser, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	f, err := l.root.Open(key)
	if errors.Is(err, os.ErrNotExist) {
		return nil, domainstorage.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open object: %w", err)
	}
	return f, nil
}

func (l *Local) Delete(_ context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if err := l.root.Remove(key); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domainstorage.ErrNotFound
		}
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

func (l *Local) Exists(_ context.Context, key string) (bool, error) {
	if err := validateKey(key); err != nil {
		return false, err
	}
	switch _, err := l.root.Stat(key); {
	case err == nil:
		return true, nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("stat object: %w", err)
	}
}
