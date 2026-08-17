package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domainstorage "github.com/streampulse/backend/internal/domain/storage"
)

func newLocal(t *testing.T) (*Local, string) {
	t.Helper()
	dir := t.TempDir()
	l, err := NewLocal(dir)
	if err != nil {
		t.Fatalf("NewLocal() error = %v", err)
	}
	return l, dir
}

func TestLocal_SaveOpenRoundTrip(t *testing.T) {
	l, _ := newLocal(t)
	ctx := context.Background()
	payload := []byte("some audio bytes")

	n, err := l.Save(ctx, "track.mp3", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if n != int64(len(payload)) {
		t.Errorf("Save() wrote %d bytes, want %d", n, len(payload))
	}

	f, err := l.Open(ctx, "track.mp3")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = f.Close() }()

	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("read %q, want %q", got, payload)
	}
}

func TestLocal_OpenIsSeekable(t *testing.T) {
	// Seekability is what lets the HTTP layer serve Range requests, so the
	// mobile player can jump inside a track instead of re-downloading it.
	l, _ := newLocal(t)
	ctx := context.Background()
	if _, err := l.Save(ctx, "t.mp3", strings.NewReader("0123456789")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	f, err := l.Open(ctx, "t.mp3")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Seek(5, io.SeekStart); err != nil {
		t.Fatalf("Seek() error = %v", err)
	}
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "56789" {
		t.Errorf("after seek read %q, want %q", got, "56789")
	}
}

func TestLocal_RejectsKeysThatEscapeTheBaseDirectory(t *testing.T) {
	// The security test of this package: no key may write outside basePath.
	l, dir := newLocal(t)
	ctx := context.Background()

	keys := []string{
		"../escaped.mp3",
		"../../etc/passwd",
		"sub/dir.mp3",
		`..\windows.mp3`,
		"/absolute.mp3",
		"",
		".",
		"..",
		"nul\x00byte.mp3",
	}

	for _, key := range keys {
		t.Run(strings.ReplaceAll(key, "\x00", "<NUL>"), func(t *testing.T) {
			if _, err := l.Save(ctx, key, strings.NewReader("x")); !errors.Is(err, ErrInvalidKey) {
				t.Errorf("Save(%q) error = %v, want ErrInvalidKey", key, err)
			}
			if _, err := l.Open(ctx, key); !errors.Is(err, ErrInvalidKey) {
				t.Errorf("Open(%q) error = %v, want ErrInvalidKey", key, err)
			}
			if err := l.Delete(ctx, key); !errors.Is(err, ErrInvalidKey) {
				t.Errorf("Delete(%q) error = %v, want ErrInvalidKey", key, err)
			}
			if _, err := l.Exists(ctx, key); !errors.Is(err, ErrInvalidKey) {
				t.Errorf("Exists(%q) error = %v, want ErrInvalidKey", key, err)
			}
		})
	}

	// Nothing was created next to (or above) the base directory.
	parent := filepath.Dir(dir)
	if _, err := os.Stat(filepath.Join(parent, "escaped.mp3")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a rejected key still managed to create a file outside the base directory")
	}
}

func TestLocal_OpenMissingKey(t *testing.T) {
	l, _ := newLocal(t)

	_, err := l.Open(context.Background(), "ghost.mp3")
	if !errors.Is(err, domainstorage.ErrNotFound) {
		t.Errorf("Open() error = %v, want storage.ErrNotFound", err)
	}
}

func TestLocal_Delete(t *testing.T) {
	l, _ := newLocal(t)
	ctx := context.Background()
	if _, err := l.Save(ctx, "gone.mp3", strings.NewReader("x")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if err := l.Delete(ctx, "gone.mp3"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := l.Delete(ctx, "gone.mp3"); !errors.Is(err, domainstorage.ErrNotFound) {
		t.Errorf("second Delete() error = %v, want storage.ErrNotFound", err)
	}
}

func TestLocal_Exists(t *testing.T) {
	l, _ := newLocal(t)
	ctx := context.Background()

	ok, err := l.Exists(ctx, "nope.mp3")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if ok {
		t.Error("Exists() = true for a missing key")
	}

	if _, err := l.Save(ctx, "here.mp3", strings.NewReader("x")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if ok, err = l.Exists(ctx, "here.mp3"); err != nil || !ok {
		t.Errorf("Exists() = %v, %v; want true, nil", ok, err)
	}
}

func TestLocal_SaveOverwritesAtomically(t *testing.T) {
	l, dir := newLocal(t)
	ctx := context.Background()

	if _, err := l.Save(ctx, "t.mp3", strings.NewReader("old")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := l.Save(ctx, "t.mp3", strings.NewReader("new content")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "t.mp3"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "new content" {
		t.Errorf("content = %q, want %q", got, "new content")
	}

	// No leftover temp files from either write.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".upload-") {
			t.Errorf("temp file %q left behind", e.Name())
		}
	}
}

// failingReader simulates a connection dropping mid-upload.
type failingReader struct{ afterBytes int }

func (r *failingReader) Read(p []byte) (int, error) {
	if r.afterBytes <= 0 {
		return 0, errors.New("connection reset")
	}
	n := min(len(p), r.afterBytes)
	r.afterBytes -= n
	return n, nil
}

func TestLocal_FailedUploadLeavesNoObject(t *testing.T) {
	// A half-written file that a listener could stream as if it were
	// complete is worse than no file at all — hence write-then-rename.
	l, dir := newLocal(t)

	if _, err := l.Save(context.Background(), "partial.mp3", &failingReader{afterBytes: 10}); err == nil {
		t.Fatal("Save() error = nil, want the read failure surfaced")
	}

	if _, err := os.Stat(filepath.Join(dir, "partial.mp3")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a failed upload left a partial object behind")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("directory is not clean after a failed upload: %v", entries)
	}
}

func TestNewLocal_CreatesTheDirectory(t *testing.T) {
	base := filepath.Join(t.TempDir(), "nested", "uploads")

	if _, err := NewLocal(base); err != nil {
		t.Fatalf("NewLocal() error = %v", err)
	}
	info, err := os.Stat(base)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Error("NewLocal did not create a directory")
	}
}
