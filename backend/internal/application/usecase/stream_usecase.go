package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
	"github.com/streampulse/backend/internal/infrastructure/streaming"
)

const (
	maxStreamTitleLen       = 120
	maxStreamDescriptionLen = 1000
)

// These are deliberately prefixed with "Stream" rather than named
// ErrForbidden/ErrInvalid: package usecase is shared by every slice, and
// ticket S1 (playlists, PR #16) already declares its own ErrForbidden there.
// Prefixing keeps this ticket mergeable whatever order the PRs land in.
// Worth unifying into a single shared error once both are on main.
var (
	// ErrStreamForbidden is returned when a user acts on a stream they do
	// not own and is not an admin.
	ErrStreamForbidden = errors.New("forbidden")
	// ErrInvalidStream is returned when stream metadata fails validation.
	ErrInvalidStream = errors.New("invalid stream")
	// ErrStreamNotLive is returned when a listener asks for a stream nobody
	// is currently broadcasting.
	ErrStreamNotLive = errors.New("stream is not live")
)

// StreamUseCase owns the lifecycle of a stream: its metadata in Postgres and
// its live session in the in-memory registry. Keeping both behind one use
// case is what stops the two from drifting apart (a row marked "live" with no
// hub behind it, or the reverse).
type StreamUseCase struct {
	streams  repository.StreamRepository
	registry *streaming.Registry
}

func NewStreamUseCase(streams repository.StreamRepository, registry *streaming.Registry) *StreamUseCase {
	return &StreamUseCase{streams: streams, registry: registry}
}

// Create registers a new stream owned by broadcasterID. A stream is created
// offline: it only goes live once someone actually starts publishing audio.
func (uc *StreamUseCase) Create(ctx context.Context, broadcasterID, title, description string) (*entity.Stream, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)

	switch {
	case title == "":
		return nil, fmt.Errorf("%w: title is required", ErrInvalidStream)
	case len(title) > maxStreamTitleLen:
		return nil, fmt.Errorf("%w: title must be at most %d characters", ErrInvalidStream, maxStreamTitleLen)
	case len(description) > maxStreamDescriptionLen:
		return nil, fmt.Errorf("%w: description must be at most %d characters", ErrInvalidStream, maxStreamDescriptionLen)
	}

	stream := &entity.Stream{
		Title:         title,
		Description:   description,
		BroadcasterID: broadcasterID,
		Status:        entity.StreamStatusOffline,
	}
	if err := uc.streams.Create(ctx, stream); err != nil {
		return nil, err
	}
	return stream, nil
}

func (uc *StreamUseCase) Get(ctx context.Context, id string) (*entity.Stream, error) {
	return uc.streams.FindByID(ctx, id)
}

func (uc *StreamUseCase) List(ctx context.Context) ([]entity.Stream, error) {
	return uc.streams.List(ctx)
}

// ListLive returns streams currently flagged live in the database, filtered
// down to those that actually have a hub in this process.
//
// The filter matters: if the API restarts while a stream is live, the row
// stays "live" but the hub is gone. Reconciling here means a listener is
// never offered a stream that would 404 the moment they tapped it.
func (uc *StreamUseCase) ListLive(ctx context.Context) ([]entity.Stream, error) {
	streams, err := uc.streams.ListByStatus(ctx, entity.StreamStatusLive)
	if err != nil {
		return nil, err
	}
	live := make([]entity.Stream, 0, len(streams))
	for _, s := range streams {
		if uc.registry.IsLive(s.ID) {
			live = append(live, s)
		}
	}
	return live, nil
}

// StartLive authorises the broadcaster, marks the stream live and opens its
// hub. The returned hub is where the caller pushes audio chunks.
func (uc *StreamUseCase) StartLive(ctx context.Context, streamID, userID, role string) (*streaming.Hub, error) {
	stream, err := uc.authorise(ctx, streamID, userID, role)
	if err != nil {
		return nil, err
	}

	if err := uc.streams.UpdateStatus(ctx, stream.ID, entity.StreamStatusLive); err != nil {
		return nil, fmt.Errorf("mark stream live: %w", err)
	}

	// The hub outlives the request context: cancelling the broadcaster's
	// request must not tear the hub down before StopLive has flipped the
	// database row back, and listeners have their own request contexts.
	return uc.registry.Open(context.WithoutCancel(ctx), stream.ID), nil
}

// StopLive closes the hub and flips the stream back to offline. It is called
// from a deferred block in the publish handlers, so it must succeed even when
// the broadcaster's request context is already cancelled.
func (uc *StreamUseCase) StopLive(ctx context.Context, streamID, userID, role string) error {
	if _, err := uc.authorise(ctx, streamID, userID, role); err != nil {
		return err
	}

	uc.registry.Close(streamID)

	if err := uc.streams.UpdateStatus(ctx, streamID, entity.StreamStatusOffline); err != nil {
		return fmt.Errorf("mark stream offline: %w", err)
	}
	return nil
}

// LiveHub returns the hub a listener should subscribe to.
func (uc *StreamUseCase) LiveHub(streamID string) (*streaming.Hub, error) {
	hub, err := uc.registry.Get(streamID)
	if err != nil {
		return nil, ErrStreamNotLive
	}
	return hub, nil
}

// ListenerCount reports the live audience of a stream (0 when offline).
func (uc *StreamUseCase) ListenerCount(streamID string) int {
	return uc.registry.ListenerCount(streamID)
}

// Delete removes a stream. Its owner or an admin may do so; a live stream is
// stopped first so listeners are released rather than left hanging.
func (uc *StreamUseCase) Delete(ctx context.Context, streamID, userID, role string) error {
	if _, err := uc.authorise(ctx, streamID, userID, role); err != nil {
		return err
	}
	uc.registry.Close(streamID)
	return uc.streams.Delete(ctx, streamID)
}

// authorise loads the stream and checks that userID owns it, or that the
// caller is an admin.
func (uc *StreamUseCase) authorise(ctx context.Context, streamID, userID, role string) (*entity.Stream, error) {
	stream, err := uc.streams.FindByID(ctx, streamID)
	if err != nil {
		return nil, err
	}
	if !stream.OwnedBy(userID) && role != string(entity.RoleAdmin) {
		return nil, ErrStreamForbidden
	}
	return stream, nil
}
