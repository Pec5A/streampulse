package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
)

var (
	// ErrForbidden is returned when a caller acts on a playlist they don't own.
	ErrForbidden = errors.New("forbidden")
	// ErrValidation is returned when input fails a business rule.
	ErrValidation = errors.New("validation failed")
	// ErrInvalidReorder is returned when the reorder list is not exactly the
	// playlist's current set of track ids (a permutation).
	ErrInvalidReorder = errors.New("reorder list must contain exactly the playlist's tracks")
)

const maxNameLen = 80

type PlaylistUseCase struct {
	repo repository.PlaylistRepository
}

func NewPlaylistUseCase(repo repository.PlaylistRepository) *PlaylistUseCase {
	return &PlaylistUseCase{repo: repo}
}

func (uc *PlaylistUseCase) Create(ctx context.Context, ownerID, name, description string, isPublic bool) (*entity.Playlist, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > maxNameLen {
		return nil, fmt.Errorf("%w: name must be 1..%d characters", ErrValidation, maxNameLen)
	}
	p := &entity.Playlist{OwnerID: ownerID, Name: name, Description: description, IsPublic: isPublic}
	if err := uc.repo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("create playlist: %w", err)
	}
	return p, nil
}

// Get returns a playlist and its ordered tracks. A private playlist is only
// visible to its owner.
func (uc *PlaylistUseCase) Get(ctx context.Context, callerID, playlistID string) (*entity.Playlist, []entity.Track, error) {
	p, err := uc.repo.FindByID(ctx, playlistID)
	if err != nil {
		return nil, nil, err
	}
	if !p.IsPublic && p.OwnerID != callerID {
		return nil, nil, ErrForbidden
	}
	tracks, err := uc.repo.ListTracks(ctx, playlistID)
	if err != nil {
		return nil, nil, fmt.Errorf("list tracks: %w", err)
	}
	return p, tracks, nil
}

func (uc *PlaylistUseCase) ListByOwner(ctx context.Context, ownerID string) ([]entity.Playlist, error) {
	return uc.repo.ListByOwner(ctx, ownerID)
}

// Update applies a partial change (nil fields are left untouched) after
// checking ownership.
func (uc *PlaylistUseCase) Update(ctx context.Context, callerID, playlistID string, name, description *string, isPublic *bool) (*entity.Playlist, error) {
	p, err := uc.ownedPlaylist(ctx, callerID, playlistID)
	if err != nil {
		return nil, err
	}
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" || len(trimmed) > maxNameLen {
			return nil, fmt.Errorf("%w: name must be 1..%d characters", ErrValidation, maxNameLen)
		}
		p.Name = trimmed
	}
	if description != nil {
		p.Description = *description
	}
	if isPublic != nil {
		p.IsPublic = *isPublic
	}
	if err := uc.repo.Update(ctx, p); err != nil {
		return nil, fmt.Errorf("update playlist: %w", err)
	}
	return p, nil
}

func (uc *PlaylistUseCase) Delete(ctx context.Context, callerID, playlistID string) error {
	if _, err := uc.ownedPlaylist(ctx, callerID, playlistID); err != nil {
		return err
	}
	return uc.repo.Delete(ctx, playlistID)
}

// AddTrack appends a track to the playlist's queue after checking ownership
// and validating the track. The repository assigns the id and position.
func (uc *PlaylistUseCase) AddTrack(ctx context.Context, callerID, playlistID string, t entity.Track) (*entity.Track, error) {
	if _, err := uc.ownedPlaylist(ctx, callerID, playlistID); err != nil {
		return nil, err
	}
	t.Title = strings.TrimSpace(t.Title)
	if t.Title == "" {
		return nil, fmt.Errorf("%w: track title is required", ErrValidation)
	}
	if t.DurationSeconds < 0 {
		return nil, fmt.Errorf("%w: duration must be >= 0", ErrValidation)
	}
	t.PlaylistID = playlistID
	if err := uc.repo.AddTrack(ctx, &t); err != nil {
		return nil, fmt.Errorf("add track: %w", err)
	}
	return &t, nil
}

func (uc *PlaylistUseCase) RemoveTrack(ctx context.Context, callerID, playlistID, trackID string) error {
	if _, err := uc.ownedPlaylist(ctx, callerID, playlistID); err != nil {
		return err
	}
	return uc.repo.RemoveTrack(ctx, playlistID, trackID)
}

// ReorderTracks validates that orderedTrackIDs is a permutation of the
// playlist's current track ids, then delegates the atomic write to the repo.
func (uc *PlaylistUseCase) ReorderTracks(ctx context.Context, callerID, playlistID string, orderedTrackIDs []string) error {
	if _, err := uc.ownedPlaylist(ctx, callerID, playlistID); err != nil {
		return err
	}
	current, err := uc.repo.ListTracks(ctx, playlistID)
	if err != nil {
		return fmt.Errorf("list tracks: %w", err)
	}
	if !isPermutation(current, orderedTrackIDs) {
		return ErrInvalidReorder
	}
	return uc.repo.ReorderTracks(ctx, playlistID, orderedTrackIDs)
}

func (uc *PlaylistUseCase) ownedPlaylist(ctx context.Context, callerID, playlistID string) (*entity.Playlist, error) {
	p, err := uc.repo.FindByID(ctx, playlistID)
	if err != nil {
		return nil, err
	}
	if p.OwnerID != callerID {
		return nil, ErrForbidden
	}
	return p, nil
}

// isPermutation reports whether ids contains exactly the track ids in tracks,
// each exactly once. Track ids are unique, so a valid reorder has no repeats.
func isPermutation(tracks []entity.Track, ids []string) bool {
	if len(tracks) != len(ids) {
		return false
	}
	a := make([]string, len(tracks))
	for i, t := range tracks {
		a[i] = t.ID
	}
	b := append([]string(nil), ids...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
