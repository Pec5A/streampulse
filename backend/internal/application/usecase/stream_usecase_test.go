package usecase

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/streampulse/backend/internal/domain/entity"
	"github.com/streampulse/backend/internal/domain/repository"
	"github.com/streampulse/backend/internal/infrastructure/streaming"
)

// fakeStreamRepo is an in-memory repository.StreamRepository. Like
// fakeUserRepo, it keeps the use-case tests free of a real database while
// still exercising the real error contract (ErrNotFound, etc.).
type fakeStreamRepo struct {
	mu      sync.Mutex
	byID    map[string]*entity.Stream
	counter int

	// failUpdateStatus makes UpdateStatus return an error, to test that the
	// use case surfaces persistence failures instead of swallowing them.
	failUpdateStatus error
}

func newFakeStreamRepo() *fakeStreamRepo {
	return &fakeStreamRepo{byID: map[string]*entity.Stream{}}
}

func (r *fakeStreamRepo) Create(_ context.Context, s *entity.Stream) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counter++
	if s.ID == "" {
		s.ID = "stream-" + string(rune('a'-1+r.counter))
	}
	clone := *s
	r.byID[s.ID] = &clone
	return nil
}

func (r *fakeStreamRepo) FindByID(_ context.Context, id string) (*entity.Stream, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.byID[id]; ok {
		clone := *s
		return &clone, nil
	}
	return nil, repository.ErrNotFound
}

func (r *fakeStreamRepo) List(_ context.Context) ([]entity.Stream, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.Stream, 0, len(r.byID))
	for _, s := range r.byID {
		out = append(out, *s)
	}
	return out, nil
}

func (r *fakeStreamRepo) ListByStatus(_ context.Context, status entity.StreamStatus) ([]entity.Stream, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]entity.Stream, 0)
	for _, s := range r.byID {
		if s.Status == status {
			out = append(out, *s)
		}
	}
	return out, nil
}

func (r *fakeStreamRepo) UpdateStatus(_ context.Context, id string, status entity.StreamStatus) error {
	if r.failUpdateStatus != nil {
		return r.failUpdateStatus
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return repository.ErrNotFound
	}
	s.Status = status
	return nil
}

func (r *fakeStreamRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return repository.ErrNotFound
	}
	delete(r.byID, id)
	return nil
}

var _ repository.StreamRepository = (*fakeStreamRepo)(nil)

func newStreamUC(t *testing.T) (*StreamUseCase, *fakeStreamRepo, *streaming.Registry) {
	t.Helper()
	uc, repo, _, reg, _ := newStreamUCWithChat(t)
	return uc, repo, reg
}

// newStreamUCWithChat is newStreamUC plus access to the user repo (to seed a
// participant for JoinChat) and the chat registry (to assert room state
// directly) — kept separate so the many existing call sites above that only
// destructure 3 values don't need touching for a dependency only the chat
// tests care about.
func newStreamUCWithChat(t *testing.T) (*StreamUseCase, *fakeStreamRepo, *fakeUserRepo, *streaming.Registry, *streaming.ChatRegistry) {
	t.Helper()
	repo := newFakeStreamRepo()
	users := newFakeUserRepo()
	reg := streaming.NewRegistry()
	chats := streaming.NewChatRegistry()
	t.Cleanup(reg.CloseAll)
	t.Cleanup(chats.CloseAll)
	return NewStreamUseCase(repo, users, reg, chats), repo, users, reg, chats
}

func mustCreateStream(t *testing.T, uc *StreamUseCase, owner string) *entity.Stream {
	t.Helper()
	s, err := uc.Create(context.Background(), owner, "Jazz de nuit", "session live")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return s
}

func TestStreamUseCase_CreateStartsOffline(t *testing.T) {
	uc, _, _ := newStreamUC(t)

	s := mustCreateStream(t, uc, "user-1")

	if s.Status != entity.StreamStatusOffline {
		t.Errorf("Status = %q, want %q", s.Status, entity.StreamStatusOffline)
	}
	if s.BroadcasterID != "user-1" {
		t.Errorf("BroadcasterID = %q, want user-1", s.BroadcasterID)
	}
	if s.ID == "" {
		t.Error("Create did not assign an ID")
	}
}

func TestStreamUseCase_CreateTrimsAndValidates(t *testing.T) {
	uc, _, _ := newStreamUC(t)
	ctx := context.Background()

	t.Run("trims whitespace", func(t *testing.T) {
		s, err := uc.Create(ctx, "user-1", "  Titre  ", "  desc  ")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if s.Title != "Titre" || s.Description != "desc" {
			t.Errorf("got title=%q desc=%q, want trimmed", s.Title, s.Description)
		}
	})

	t.Run("rejects an empty title", func(t *testing.T) {
		if _, err := uc.Create(ctx, "user-1", "   ", ""); !errors.Is(err, ErrInvalidStream) {
			t.Errorf("error = %v, want ErrInvalidStream", err)
		}
	})

	t.Run("rejects an over-long title", func(t *testing.T) {
		_, err := uc.Create(ctx, "user-1", strings.Repeat("a", maxStreamTitleLen+1), "")
		if !errors.Is(err, ErrInvalidStream) {
			t.Errorf("error = %v, want ErrInvalidStream", err)
		}
	})

	t.Run("accepts a title at the limit", func(t *testing.T) {
		if _, err := uc.Create(ctx, "user-1", strings.Repeat("a", maxStreamTitleLen), ""); err != nil {
			t.Errorf("Create() error = %v, want nil at the boundary", err)
		}
	})

	t.Run("rejects an over-long description", func(t *testing.T) {
		_, err := uc.Create(ctx, "user-1", "ok", strings.Repeat("a", maxStreamDescriptionLen+1))
		if !errors.Is(err, ErrInvalidStream) {
			t.Errorf("error = %v, want ErrInvalidStream", err)
		}
	})
}

func TestStreamUseCase_StartLiveOpensAHubAndMarksTheRowLive(t *testing.T) {
	uc, repo, reg := newStreamUC(t)
	s := mustCreateStream(t, uc, "user-1")

	hub, err := uc.StartLive(context.Background(), s.ID, "user-1", string(entity.RoleUser))
	if err != nil {
		t.Fatalf("StartLive() error = %v", err)
	}
	if hub == nil {
		t.Fatal("StartLive returned a nil hub")
	}
	if !reg.IsLive(s.ID) {
		t.Error("registry does not report the stream as live")
	}

	stored, _ := repo.FindByID(context.Background(), s.ID)
	if stored.Status != entity.StreamStatusLive {
		t.Errorf("persisted status = %q, want live", stored.Status)
	}
}

func TestStreamUseCase_StartLiveSurvivesRequestCancellation(t *testing.T) {
	// The publish handler's request context dies the instant the broadcaster
	// disconnects. The hub must not be torn down by that — StopLive is what
	// ends a stream, via a deferred call that still needs the hub around.
	uc, _, _ := newStreamUC(t)
	s := mustCreateStream(t, uc, "user-1")

	ctx, cancel := context.WithCancel(context.Background())
	hub, err := uc.StartLive(ctx, s.ID, "user-1", string(entity.RoleUser))
	if err != nil {
		t.Fatalf("StartLive() error = %v", err)
	}
	cancel()

	select {
	case <-hub.Done():
		t.Error("hub was closed by the broadcaster's request context")
	default:
	}
}

func TestStreamUseCase_StartLiveRejectsNonOwners(t *testing.T) {
	uc, _, reg := newStreamUC(t)
	s := mustCreateStream(t, uc, "user-1")

	_, err := uc.StartLive(context.Background(), s.ID, "attacker", string(entity.RoleUser))
	if !errors.Is(err, ErrStreamForbidden) {
		t.Errorf("error = %v, want ErrStreamForbidden", err)
	}
	if reg.IsLive(s.ID) {
		t.Error("a rejected StartLive still opened a hub")
	}
}

func TestStreamUseCase_AdminMayControlAnyStream(t *testing.T) {
	uc, _, _ := newStreamUC(t)
	s := mustCreateStream(t, uc, "user-1")

	if _, err := uc.StartLive(context.Background(), s.ID, "moderator", string(entity.RoleAdmin)); err != nil {
		t.Fatalf("admin StartLive() error = %v", err)
	}
	if err := uc.StopLive(context.Background(), s.ID, "moderator", string(entity.RoleAdmin)); err != nil {
		t.Fatalf("admin StopLive() error = %v", err)
	}
}

func TestStreamUseCase_StartLiveOnUnknownStream(t *testing.T) {
	uc, _, _ := newStreamUC(t)

	_, err := uc.StartLive(context.Background(), "ghost", "user-1", string(entity.RoleUser))
	if !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestStreamUseCase_StartLiveSurfacesPersistenceFailures(t *testing.T) {
	uc, repo, reg := newStreamUC(t)
	s := mustCreateStream(t, uc, "user-1")
	repo.failUpdateStatus = errors.New("db down")

	if _, err := uc.StartLive(context.Background(), s.ID, "user-1", string(entity.RoleUser)); err == nil {
		t.Fatal("StartLive() error = nil, want the persistence failure surfaced")
	}
	if reg.IsLive(s.ID) {
		t.Error("hub was opened even though the status update failed")
	}
}

func TestStreamUseCase_StopLiveClosesTheHubAndListeners(t *testing.T) {
	uc, repo, reg := newStreamUC(t)
	s := mustCreateStream(t, uc, "user-1")

	hub, err := uc.StartLive(context.Background(), s.ID, "user-1", string(entity.RoleUser))
	if err != nil {
		t.Fatalf("StartLive() error = %v", err)
	}
	_, ch, _, err := hub.Subscribe()
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	if err := uc.StopLive(context.Background(), s.ID, "user-1", string(entity.RoleUser)); err != nil {
		t.Fatalf("StopLive() error = %v", err)
	}

	if _, ok := <-ch; ok {
		t.Error("listener channel still open after StopLive")
	}
	if reg.IsLive(s.ID) {
		t.Error("registry still reports the stream live after StopLive")
	}
	stored, _ := repo.FindByID(context.Background(), s.ID)
	if stored.Status != entity.StreamStatusOffline {
		t.Errorf("persisted status = %q, want offline", stored.Status)
	}
}

func TestStreamUseCase_StopLiveRejectsNonOwners(t *testing.T) {
	uc, _, reg := newStreamUC(t)
	s := mustCreateStream(t, uc, "user-1")
	if _, err := uc.StartLive(context.Background(), s.ID, "user-1", string(entity.RoleUser)); err != nil {
		t.Fatalf("StartLive() error = %v", err)
	}

	err := uc.StopLive(context.Background(), s.ID, "attacker", string(entity.RoleUser))
	if !errors.Is(err, ErrStreamForbidden) {
		t.Errorf("error = %v, want ErrStreamForbidden", err)
	}
	if !reg.IsLive(s.ID) {
		t.Error("a rejected StopLive still killed the stream")
	}
}

func TestStreamUseCase_LiveHub(t *testing.T) {
	uc, _, _ := newStreamUC(t)
	s := mustCreateStream(t, uc, "user-1")

	if _, err := uc.LiveHub(s.ID); !errors.Is(err, ErrStreamNotLive) {
		t.Errorf("LiveHub on an offline stream = %v, want ErrStreamNotLive", err)
	}

	started, err := uc.StartLive(context.Background(), s.ID, "user-1", string(entity.RoleUser))
	if err != nil {
		t.Fatalf("StartLive() error = %v", err)
	}
	got, err := uc.LiveHub(s.ID)
	if err != nil {
		t.Fatalf("LiveHub() error = %v", err)
	}
	if got != started {
		t.Error("LiveHub returned a different hub than StartLive")
	}
}

func TestStreamUseCase_ListLiveHidesRowsWithoutAHub(t *testing.T) {
	// Simulates an API restart: the row says "live" but no hub survived.
	// Listeners must not be offered a stream that would immediately 404.
	uc, repo, _ := newStreamUC(t)
	ctx := context.Background()

	stale := mustCreateStream(t, uc, "user-1")
	if err := repo.UpdateStatus(ctx, stale.ID, entity.StreamStatusLive); err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	real := mustCreateStream(t, uc, "user-2")
	if _, err := uc.StartLive(ctx, real.ID, "user-2", string(entity.RoleUser)); err != nil {
		t.Fatalf("StartLive() error = %v", err)
	}

	live, err := uc.ListLive(ctx)
	if err != nil {
		t.Fatalf("ListLive() error = %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("ListLive returned %d streams, want 1", len(live))
	}
	if live[0].ID != real.ID {
		t.Errorf("ListLive returned %q, want the stream with a live hub (%q)", live[0].ID, real.ID)
	}
}

func TestStreamUseCase_ListenerCount(t *testing.T) {
	uc, _, _ := newStreamUC(t)
	s := mustCreateStream(t, uc, "user-1")

	if got := uc.ListenerCount(s.ID); got != 0 {
		t.Errorf("ListenerCount on an offline stream = %d, want 0", got)
	}

	hub, err := uc.StartLive(context.Background(), s.ID, "user-1", string(entity.RoleUser))
	if err != nil {
		t.Fatalf("StartLive() error = %v", err)
	}
	for range 4 {
		if _, _, _, err := hub.Subscribe(); err != nil {
			t.Fatalf("Subscribe() error = %v", err)
		}
	}
	if got := uc.ListenerCount(s.ID); got != 4 {
		t.Errorf("ListenerCount = %d, want 4", got)
	}
}

func TestStreamUseCase_DeleteStopsTheStreamFirst(t *testing.T) {
	uc, repo, reg := newStreamUC(t)
	ctx := context.Background()
	s := mustCreateStream(t, uc, "user-1")

	hub, err := uc.StartLive(ctx, s.ID, "user-1", string(entity.RoleUser))
	if err != nil {
		t.Fatalf("StartLive() error = %v", err)
	}
	_, ch, _, err := hub.Subscribe()
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	if err := uc.Delete(ctx, s.ID, "user-1", string(entity.RoleUser)); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, ok := <-ch; ok {
		t.Error("listener was left hanging on a deleted stream")
	}
	if reg.IsLive(s.ID) {
		t.Error("registry still reports a deleted stream as live")
	}
	if _, err := repo.FindByID(ctx, s.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("stream still in the repository after Delete: %v", err)
	}
}

func TestStreamUseCase_DeleteRejectsNonOwners(t *testing.T) {
	uc, repo, _ := newStreamUC(t)
	s := mustCreateStream(t, uc, "user-1")

	if err := uc.Delete(context.Background(), s.ID, "attacker", string(entity.RoleUser)); !errors.Is(err, ErrStreamForbidden) {
		t.Errorf("error = %v, want ErrStreamForbidden", err)
	}
	if _, err := repo.FindByID(context.Background(), s.ID); err != nil {
		t.Errorf("stream was deleted despite the rejection: %v", err)
	}
}

func TestStreamUseCase_GetAndList(t *testing.T) {
	uc, _, _ := newStreamUC(t)
	ctx := context.Background()
	s := mustCreateStream(t, uc, "user-1")

	got, err := uc.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != s.ID {
		t.Errorf("Get returned %q, want %q", got.ID, s.ID)
	}

	if _, err := uc.Get(ctx, "ghost"); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Get on an unknown id = %v, want ErrNotFound", err)
	}

	all, err := uc.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) != 1 {
		t.Errorf("List returned %d streams, want 1", len(all))
	}
}

func TestStreamUseCase_StartLiveOpensTheChatRoomTogetherWithTheHub(t *testing.T) {
	uc, _, _, _, chats := newStreamUCWithChat(t)
	s := mustCreateStream(t, uc, "user-1")

	if _, err := uc.StartLive(context.Background(), s.ID, "user-1", string(entity.RoleUser)); err != nil {
		t.Fatalf("StartLive() error = %v", err)
	}

	if _, err := chats.Get(s.ID); err != nil {
		t.Errorf("chat room not opened by StartLive: Get() error = %v", err)
	}
}

func TestStreamUseCase_StopLiveClosesTheChatRoomToo(t *testing.T) {
	uc, _, _, _, chats := newStreamUCWithChat(t)
	s := mustCreateStream(t, uc, "user-1")
	if _, err := uc.StartLive(context.Background(), s.ID, "user-1", string(entity.RoleUser)); err != nil {
		t.Fatalf("StartLive() error = %v", err)
	}

	if err := uc.StopLive(context.Background(), s.ID, "user-1", string(entity.RoleUser)); err != nil {
		t.Fatalf("StopLive() error = %v", err)
	}

	if _, err := chats.Get(s.ID); err != streaming.ErrStreamNotFound {
		t.Errorf("chat room not closed by StopLive: Get() error = %v, want ErrStreamNotFound", err)
	}
}

func TestStreamUseCase_DeleteClosesTheChatRoomToo(t *testing.T) {
	uc, _, _, _, chats := newStreamUCWithChat(t)
	s := mustCreateStream(t, uc, "user-1")
	if _, err := uc.StartLive(context.Background(), s.ID, "user-1", string(entity.RoleUser)); err != nil {
		t.Fatalf("StartLive() error = %v", err)
	}

	if err := uc.Delete(context.Background(), s.ID, "user-1", string(entity.RoleUser)); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := chats.Get(s.ID); err != streaming.ErrStreamNotFound {
		t.Errorf("chat room not closed by Delete: Get() error = %v, want ErrStreamNotFound", err)
	}
}

func TestStreamUseCase_JoinChatResolvesTheParticipantsUsername(t *testing.T) {
	uc, _, users, _, _ := newStreamUCWithChat(t)
	s := mustCreateStream(t, uc, "user-1")
	if _, err := uc.StartLive(context.Background(), s.ID, "user-1", string(entity.RoleUser)); err != nil {
		t.Fatalf("StartLive() error = %v", err)
	}

	participant := &entity.User{Email: "listener@b.com", Username: "chatty"}
	if err := users.Create(context.Background(), participant); err != nil {
		t.Fatalf("seed participant: %v", err)
	}

	hub, username, err := uc.JoinChat(context.Background(), s.ID, participant.ID)
	if err != nil {
		t.Fatalf("JoinChat() error = %v", err)
	}
	if hub == nil {
		t.Fatal("JoinChat returned a nil hub")
	}
	if username != "chatty" {
		t.Errorf("username = %q, want chatty", username)
	}
}

func TestStreamUseCase_JoinChatOnAnOfflineStream(t *testing.T) {
	// The stream exists but nobody has started broadcasting yet, so no chat
	// room exists — joining must fail the same way LiveHub does for a
	// listener, not with some other opaque error.
	uc, _, users, _, _ := newStreamUCWithChat(t)
	s := mustCreateStream(t, uc, "user-1")

	participant := &entity.User{Email: "listener@b.com", Username: "chatty"}
	if err := users.Create(context.Background(), participant); err != nil {
		t.Fatalf("seed participant: %v", err)
	}

	if _, _, err := uc.JoinChat(context.Background(), s.ID, participant.ID); !errors.Is(err, ErrStreamNotLive) {
		t.Errorf("JoinChat() on an offline stream error = %v, want ErrStreamNotLive", err)
	}
}

func TestStreamUseCase_JoinChatDoesNotRequireOwnership(t *testing.T) {
	// Unlike StartLive/StopLive, any authenticated user may join and post in
	// a live stream's chat — the same openness as Listen for audio.
	uc, _, users, _, _ := newStreamUCWithChat(t)
	s := mustCreateStream(t, uc, "owner")
	if _, err := uc.StartLive(context.Background(), s.ID, "owner", string(entity.RoleUser)); err != nil {
		t.Fatalf("StartLive() error = %v", err)
	}

	stranger := &entity.User{Email: "stranger@b.com", Username: "rando"}
	if err := users.Create(context.Background(), stranger); err != nil {
		t.Fatalf("seed stranger: %v", err)
	}

	if _, _, err := uc.JoinChat(context.Background(), s.ID, stranger.ID); err != nil {
		t.Errorf("JoinChat() by a non-owner error = %v, want nil", err)
	}
}

func TestStreamUseCase_JoinChatWithAnUnknownUser(t *testing.T) {
	uc, _, _, _, _ := newStreamUCWithChat(t)
	s := mustCreateStream(t, uc, "user-1")
	if _, err := uc.StartLive(context.Background(), s.ID, "user-1", string(entity.RoleUser)); err != nil {
		t.Fatalf("StartLive() error = %v", err)
	}

	if _, _, err := uc.JoinChat(context.Background(), s.ID, "ghost"); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("JoinChat() with an unknown user error = %v, want ErrNotFound", err)
	}
}
