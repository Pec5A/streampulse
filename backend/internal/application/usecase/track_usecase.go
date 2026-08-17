package usecase

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
	"github.com/streampulse/backend/internal/domain/storage"
)

const (
	// MaxUploadBytes caps a single upload. Enforced by actually counting
	// bytes as they stream through — a declared Content-Length is a claim,
	// not a fact, and can simply be a lie.
	MaxUploadBytes = 50 << 20 // 50 MiB

	// sniffLen is what http.DetectContentType inspects.
	sniffLen = 512

	maxTrackTitleLen  = 200
	maxTrackArtistLen = 200
)

var (
	// ErrUnsupportedMedia is returned when the upload is not an accepted
	// audio format, or when its content looks like something dangerous.
	ErrUnsupportedMedia = errors.New("unsupported media type")
	// ErrFileTooLarge is returned when the upload exceeds MaxUploadBytes.
	ErrFileTooLarge = errors.New("file too large")
	// ErrEmptyFile is returned for a zero-byte upload.
	ErrEmptyFile = errors.New("file is empty")
	// ErrInvalidTrack is returned when track metadata fails validation.
	ErrInvalidTrack = errors.New("invalid track")
)

// acceptedAudioTypes maps the Content-Type a client may declare to the
// extension we store it under.
var acceptedAudioTypes = map[string]string{
	"audio/mpeg":  ".mp3",
	"audio/mp3":   ".mp3",
	"audio/aac":   ".aac",
	"audio/x-aac": ".aac",
	"audio/mp4":   ".m4a",
	"audio/x-m4a": ".m4a",
	"audio/ogg":   ".ogg",
	"audio/opus":  ".opus",
	"audio/flac":  ".flac",
	"audio/wav":   ".wav",
	"audio/wave":  ".wav",
	"audio/x-wav": ".wav",
}

// activeContentTypes are the sniffed types we refuse outright: these are the
// ones a browser would execute or render if the file were ever served back
// inline. Anything here is a stored-XSS attempt wearing an .mp3 extension.
var activeContentTypes = []string{
	"text/html",
	"text/xml",
	"application/xml",
	"image/svg+xml",
	"application/javascript",
	"text/javascript",
	"application/x-shockwave-flash",
	"application/pdf",
}

// TrackUseCase handles uploading and managing audio files.
type TrackUseCase struct {
	tracks  repository.TrackRepository
	storage storage.Storage
}

func NewTrackUseCase(tracks repository.TrackRepository, store storage.Storage) *TrackUseCase {
	return &TrackUseCase{tracks: tracks, storage: store}
}

// UploadInput carries everything the transport layer extracted from the
// multipart request.
type UploadInput struct {
	UploaderID  string
	Title       string
	Artist      string
	Filename    string
	ContentType string
	Body        io.Reader
}

// Upload validates and stores an audio file, then records its metadata.
//
// Two things are deliberately not trusted: the declared Content-Type and the
// declared size. The declared type only selects the extension; the *actual*
// first bytes are sniffed and rejected if they look like active content. The
// size is counted while streaming, never read from a header.
func (uc *TrackUseCase) Upload(ctx context.Context, in UploadInput) (*entity.AudioTrack, error) {
	title := strings.TrimSpace(in.Title)
	artist := strings.TrimSpace(in.Artist)

	switch {
	case title == "":
		return nil, fmt.Errorf("%w: title is required", ErrInvalidTrack)
	case len(title) > maxTrackTitleLen:
		return nil, fmt.Errorf("%w: title must be at most %d characters", ErrInvalidTrack, maxTrackTitleLen)
	case len(artist) > maxTrackArtistLen:
		return nil, fmt.Errorf("%w: artist must be at most %d characters", ErrInvalidTrack, maxTrackArtistLen)
	}

	declared := normaliseContentType(in.ContentType)
	ext, ok := acceptedAudioTypes[declared]
	if !ok {
		return nil, fmt.Errorf("%w: %q is not an accepted audio type", ErrUnsupportedMedia, declared)
	}

	// Peek at the head to sniff the real content, then stitch it back in
	// front of the rest so nothing is buffered beyond these 512 bytes.
	head := make([]byte, sniffLen)
	n, err := io.ReadFull(in.Body, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("read upload: %w", err)
	}
	head = head[:n]
	if n == 0 {
		return nil, ErrEmptyFile
	}
	if err := rejectActiveContent(head); err != nil {
		return nil, err
	}

	// LimitReader takes one byte more than the cap so that hitting exactly
	// the limit is distinguishable from exceeding it.
	body := io.MultiReader(bytes.NewReader(head), io.LimitReader(in.Body, MaxUploadBytes+1))

	key := uuid.NewString() + ext
	written, err := uc.storage.Save(ctx, key, body)
	if err != nil {
		return nil, fmt.Errorf("store upload: %w", err)
	}
	if written > MaxUploadBytes {
		// Clean up rather than leaving an oversized object behind.
		_ = uc.storage.Delete(context.WithoutCancel(ctx), key)
		return nil, ErrFileTooLarge
	}

	track := &entity.AudioTrack{
		Title:       title,
		Artist:      artist,
		StorageKey:  key,
		Filename:    sanitiseFilename(in.Filename),
		ContentType: declared,
		SizeBytes:   written,
		UploaderID:  in.UploaderID,
	}
	if err := uc.tracks.Create(ctx, track); err != nil {
		// The row is the source of truth; an object with no row is
		// unreachable garbage, so drop it.
		_ = uc.storage.Delete(context.WithoutCancel(ctx), key)
		return nil, fmt.Errorf("persist track: %w", err)
	}
	return track, nil
}

// Open returns the stored audio for a track, for the download/stream route.
func (uc *TrackUseCase) Open(ctx context.Context, id string) (*entity.AudioTrack, io.ReadSeekCloser, error) {
	track, err := uc.tracks.FindByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	f, err := uc.storage.Open(ctx, track.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			// Row without object: the metadata says it exists but the bytes
			// are gone. Surface it as not-found rather than a 500.
			return nil, nil, repository.ErrNotFound
		}
		return nil, nil, fmt.Errorf("open track: %w", err)
	}
	return track, f, nil
}

func (uc *TrackUseCase) Get(ctx context.Context, id string) (*entity.AudioTrack, error) {
	return uc.tracks.FindByID(ctx, id)
}

func (uc *TrackUseCase) List(ctx context.Context, limit, offset int) ([]entity.AudioTrack, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return uc.tracks.List(ctx, limit, offset)
}

func (uc *TrackUseCase) ListMine(ctx context.Context, uploaderID string) ([]entity.AudioTrack, error) {
	return uc.tracks.ListByUploader(ctx, uploaderID)
}

// Delete removes a track and its stored object. Only the uploader or an
// admin may do so.
func (uc *TrackUseCase) Delete(ctx context.Context, id, callerID, callerRole string) error {
	track, err := uc.tracks.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !track.OwnedBy(callerID) && callerRole != string(entity.RoleAdmin) {
		return ErrStreamForbidden
	}

	// Row first: if the object delete fails we are left with an orphaned
	// file (recoverable, invisible to users) rather than a row pointing at
	// nothing (a broken track in every listing).
	if err := uc.tracks.Delete(ctx, id); err != nil {
		return err
	}
	if err := uc.storage.Delete(ctx, track.StorageKey); err != nil && !errors.Is(err, storage.ErrNotFound) {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

// rejectActiveContent refuses uploads whose real bytes look like something a
// browser would execute or render.
//
// Why sniff to *reject* rather than to whitelist: Go's http.DetectContentType
// only recognises a handful of audio containers — an MP3 with no ID3 tag, an
// AAC or a FLAC all sniff as application/octet-stream. Whitelisting on the
// sniff would therefore reject perfectly valid audio. What actually matters
// for safety is that the file is not active content, and that is exactly what
// the sniffer is reliable at spotting.
func rejectActiveContent(head []byte) error {
	sniffed := normaliseContentType(http.DetectContentType(head))
	for _, bad := range activeContentTypes {
		if sniffed == bad {
			return fmt.Errorf("%w: content is %s, not audio", ErrUnsupportedMedia, sniffed)
		}
	}
	return nil
}

func normaliseContentType(ct string) string {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.ToLower(strings.TrimSpace(ct))
}

// sanitiseFilename keeps a displayable name and strips anything that could be
// read as a path. Defence in depth: this value never reaches the filesystem
// (the storage key is a generated UUID), but it does reach a
// Content-Disposition header.
func sanitiseFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	if name == "." || name == ".." || name == "/" {
		return ""
	}
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' {
			return -1
		}
		return r
	}, name)
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}
